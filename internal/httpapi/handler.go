package httpapi

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/seppedelanghe/roxy/internal/backend"
	"github.com/seppedelanghe/roxy/internal/cachekey"
	"github.com/seppedelanghe/roxy/internal/diskcache"
	"github.com/seppedelanghe/roxy/internal/raw"
	"github.com/seppedelanghe/roxy/internal/vipsproc"
)

// Handler wires together all processing dependencies.
type Handler struct {
	Backend      backend.Backend
	Cache        *diskcache.Cache
	RAW          *raw.Adapter
	Vips         *vipsproc.Pipeline
	Log          *slog.Logger
	MaxInputSize int64
	CacheMaxAge  int
	SlowGate     chan struct{}
	SlowWait     time.Duration
	Healthy      func() bool
	Scratch      string
	// AsyncWG tracks in-flight async cache-write goroutines.
	// When non-nil, server.Run waits for it to reach zero before sweeping tmp files.
	// Unit tests that don't set it keep working because all Add/Done calls are guarded.
	AsyncWG *sync.WaitGroup
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	if h.Healthy != nil && !h.Healthy() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, "draining")
		return
	}
	_, _ = io.WriteString(w, "ok")
}

func (h *Handler) Process(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	logLine := func(status int, pathTaken, cacheStatus string, bytesOut int, errMsg, errCode string) {
		attrs := []any{
			"method", r.Method, "path", r.URL.Path, "query", r.URL.RawQuery,
			"status", status, "duration_ms", time.Since(start).Milliseconds(),
			"path_taken", pathTaken, "cache", cacheStatus, "bytes_out", bytesOut,
			"backend", h.Backend.ID(),
		}
		if errMsg != "" {
			attrs = append(attrs, "error", errMsg, "error_code", errCode)
			h.Log.Error("request", attrs...)
			return
		}
		h.Log.Info("request", attrs...)
	}

	req, err := ParseRequest(r.URL.Query())
	if err != nil {
		st, code := writeError(w, err, err.Error())
		logLine(st, "", "", 0, err.Error(), code)
		return
	}

	format, ok := Negotiate(r.Header.Get("Accept"))
	if !ok {
		st, code := writeError(w, ErrUnsupportedFormat, "no supported Accept type")
		logLine(st, "", "", 0, "no supported Accept", code)
		return
	}

	stat, err := h.Backend.Stat(r.Context(), req.File)
	if errors.Is(err, backend.ErrNotFound) {
		st, code := writeError(w, ErrNotFound, "source not found")
		logLine(st, "", "", 0, "not found", code)
		return
	}
	if errors.Is(err, backend.ErrInvalidKey) {
		st, code := writeError(w, ErrInvalidFileKey, "invalid file key")
		logLine(st, "", "", 0, "invalid key", code)
		return
	}
	if err != nil {
		st, code := writeError(w, ErrInternal, "stat failed")
		logLine(st, "", "", 0, err.Error(), code)
		return
	}
	if stat.Size > h.MaxInputSize {
		st, code := writeError(w, ErrInputTooLarge, fmt.Sprintf("source exceeds %d bytes", h.MaxInputSize))
		logLine(st, "", "", 0, "too large", code)
		return
	}

	key := cachekey.Compute(cachekey.Input{
		BackendID:   h.Backend.ID(),
		File:        req.File,
		SourceSize:  stat.Size,
		SourceMTime: stat.ModTime,
		Res:         req.Res,
		Format:      format,
		WB:          req.WB,
		Exp:         req.Exp,
		HalfSize:    req.HalfSize,
		EmbedOnly:   req.EmbedOnly,
	})

	if inm := r.Header.Get("If-None-Match"); inm == `"`+key+`"` || inm == key {
		h.writeCacheHeaders(w, key)
		w.WriteHeader(http.StatusNotModified)
		logLine(http.StatusNotModified, "cache", "hit", 0, "", "")
		return
	}

	if f, meta, ok := h.Cache.Open(key); ok {
		defer f.Close()
		h.writeCacheHeaders(w, key)
		w.Header().Set("Content-Type", contentTypeFor(format))
		w.Header().Set("Content-Length", strconv.FormatInt(meta.Size, 10))
		w.Header().Set("X-Cache", "HIT")
		w.Header().Set("X-Path", "cache")
		n, _ := io.Copy(w, f)
		logLine(http.StatusOK, "cache", "hit", int(n), "", "")
		return
	}

	body, pathTaken, perr := h.process(r.Context(), req, format)
	if perr != nil {
		if errors.Is(perr.err, ErrAtCapacity) {
			// Retry-After jitter in [1,5] seconds, per spec §7.
			// Set BEFORE writeError so the header survives.
			w.Header().Set("Retry-After", strconv.Itoa(1+rand.IntN(5)))
		}
		st, code := writeError(w, perr.err, perr.msg)
		logLine(st, pathTaken, "miss", 0, perr.msg, code)
		return
	}

	h.writeCacheHeaders(w, key)
	w.Header().Set("Content-Type", contentTypeFor(format))
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("X-Cache", "MISS")
	w.Header().Set("X-Path", pathTaken)
	if _, err := w.Write(body); err != nil {
		logLine(http.StatusOK, pathTaken, "miss", 0, err.Error(), "write_failed")
		return
	}

	if h.AsyncWG != nil {
		h.AsyncWG.Add(1)
	}
	go func(buf []byte) {
		if h.AsyncWG != nil {
			defer h.AsyncWG.Done()
		}
		if err := h.Cache.Store(key, buf, contentTypeFor(format)); err != nil {
			h.Log.Warn("cache_store_failed", "key", key, "err", err)
		}
	}(body)

	logLine(http.StatusOK, pathTaken, "miss", len(body), "", "")
}

func (h *Handler) writeCacheHeaders(w http.ResponseWriter, key string) {
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", h.CacheMaxAge))
	w.Header().Set("ETag", `"`+key+`"`)
	w.Header().Set("Vary", "Accept")
}

func contentTypeFor(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "webp":
		return "image/webp"
	}
	return "application/octet-stream"
}

type processError struct {
	err error
	msg string
}

func (h *Handler) process(ctx context.Context, req Request, format string) ([]byte, string, *processError) {
	localPath, cleanup, perr := h.materialize(ctx, req.File)
	if perr != nil {
		return nil, "", perr
	}
	defer cleanup()

	wantFast := req.WB == "auto" && req.Exp == 0
	if wantFast {
		data, info, err := h.RAW.ExtractLargestPreview(ctx, localPath)
		if err == nil {
			if req.Preset.LongestEdge() == 0 || info.LongestEdge >= req.Preset.LongestEdge() {
				out, _, vErr := h.Vips.ProcessJPEG(data, vipsOpts(req, format))
				if vErr != nil {
					return nil, "fast", &processError{ErrInternal, vErr.Error()}
				}
				return out, "fast", nil
			}
		} else if !errors.Is(err, raw.ErrNoPreview) {
			return nil, "fast", &processError{ErrInternal, err.Error()}
		}
		if req.EmbedOnly {
			return nil, "fast", &processError{ErrNoEmbeddedPreview, "no embedded preview"}
		}
	}

	select {
	case h.SlowGate <- struct{}{}:
	default:
		t := time.NewTimer(h.SlowWait)
		select {
		case h.SlowGate <- struct{}{}:
			t.Stop()
		case <-t.C:
			return nil, "slow", &processError{ErrAtCapacity, "slow path saturated"}
		case <-ctx.Done():
			t.Stop()
			return nil, "slow", &processError{ErrInternal, ctx.Err().Error()}
		}
	}
	defer func() { <-h.SlowGate }()

	img, _, dErr := h.RAW.Demosaic(ctx, localPath, raw.DemosaicOptions{
		HalfSize:      req.HalfSize,
		WhiteBalance:  req.WB,
		ExposureStops: req.Exp,
	})
	if dErr != nil {
		return nil, "slow", &processError{ErrUnsupportedFormat, dErr.Error()}
	}
	out, _, vErr := h.Vips.ProcessImage(img, vipsOpts(req, format))
	if vErr != nil {
		return nil, "slow", &processError{ErrInternal, vErr.Error()}
	}
	return out, "slow", nil
}

func vipsOpts(req Request, format string) vipsproc.Options {
	return vipsproc.Options{
		Width:  req.Preset.Width,
		Height: req.Preset.Height,
		Fit:    vipsproc.Fit(req.Preset.Fit),
		Format: vipsproc.Format(format),
	}
}

func (h *Handler) materialize(ctx context.Context, key string) (string, func(), *processError) {
	obj, stat, err := h.Backend.Open(ctx, key)
	if err != nil {
		if errors.Is(err, backend.ErrNotFound) {
			return "", func() {}, &processError{ErrNotFound, "not found"}
		}
		if errors.Is(err, backend.ErrInvalidKey) {
			return "", func() {}, &processError{ErrInvalidFileKey, "invalid key"}
		}
		return "", func() {}, &processError{ErrInternal, err.Error()}
	}
	// If the backend gave us a real *os.File, use it directly.
	if f, ok := obj.(*os.File); ok {
		path := f.Name()
		return path, func() { f.Close() }, nil
	}
	// Otherwise stream to a scratch temp file.
	scratch := h.Scratch
	if scratch == "" {
		scratch = os.TempDir()
	}
	var nb [8]byte
	_, _ = crand.Read(nb[:])
	tmp := filepath.Join(scratch, "roxy-"+hex.EncodeToString(nb[:])+".raw")
	out, err := os.Create(tmp)
	if err != nil {
		obj.Close()
		return "", func() {}, &processError{ErrInternal, err.Error()}
	}
	// Stream straight to disk in fixed-size chunks rather than buffering the
	// whole source in memory (sources can be up to MaxInputSize).
	if _, err := io.Copy(out, io.NewSectionReader(obj, 0, stat.Size)); err != nil {
		out.Close()
		obj.Close()
		os.Remove(tmp)
		return "", func() {}, &processError{ErrInternal, err.Error()}
	}
	out.Close()
	obj.Close()
	return tmp, func() { os.Remove(tmp) }, nil
}
