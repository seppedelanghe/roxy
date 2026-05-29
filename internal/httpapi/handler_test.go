package httpapi

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/seppedelanghe/roxy/internal/backend"
	"github.com/seppedelanghe/roxy/internal/diskcache"
	"github.com/seppedelanghe/roxy/internal/logging"
	"github.com/seppedelanghe/roxy/internal/vipsproc"
)

func newTestHandler(t *testing.T) (*Handler, string) {
	root := t.TempDir()
	cacheDir := t.TempDir()
	b, err := backend.NewLocal(root)
	require.NoError(t, err)
	lru := diskcache.NewLRU(0)
	cache := diskcache.New(cacheDir, lru)
	wg := &sync.WaitGroup{}
	t.Cleanup(wg.Wait)
	h := &Handler{
		Backend:      b,
		Cache:        cache,
		Log:          logging.NewTo(os.Stderr, "error"),
		MaxInputSize: 100 << 20,
		CacheMaxAge:  60,
		SlowGate:     make(chan struct{}, 1),
		SlowWait:     0,
		Healthy:      func() bool { return true },
		AsyncWG:      wg,
	}
	return h, root
}

func TestHealthz(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Healthz(rec, httptest.NewRequest("GET", "/healthz", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "ok", rec.Body.String())
}

func TestHealthzDraining(t *testing.T) {
	h, _ := newTestHandler(t)
	h.Healthy = func() bool { return false }
	rec := httptest.NewRecorder()
	h.Healthz(rec, httptest.NewRequest("GET", "/healthz", nil))
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Equal(t, "draining", rec.Body.String())
}

func TestProcessNotFound(t *testing.T) {
	h, _ := newTestHandler(t)
	q := url.Values{}
	q.Set("file", "missing.NEF")
	rec := httptest.NewRecorder()
	h.Process(rec, httptest.NewRequest("GET", "/process?"+q.Encode(), nil))
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "not_found")
}

func TestProcessRejectsUnknownParam(t *testing.T) {
	h, _ := newTestHandler(t)
	rec := httptest.NewRecorder()
	h.Process(rec, httptest.NewRequest("GET", "/process?file=x&zzz=1", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestProcessTooLarge(t *testing.T) {
	h, root := newTestHandler(t)
	h.MaxInputSize = 4
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.bin"), []byte("12345"), 0644))
	rec := httptest.NewRecorder()
	h.Process(rec, httptest.NewRequest("GET", "/process?file=big.bin", nil))
	require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func writeJPEGFixture(t *testing.T, path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func writePNGFixture(t *testing.T, path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func TestProcessDirectJPEG(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writeJPEGFixture(t, filepath.Join(root, "photo.jpg"), 1600, 900)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=photo.jpg&res=720p", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
	require.Equal(t, "MISS", rec.Header().Get("X-Cache"))
}

func TestProcessDirectPNG(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writePNGFixture(t, filepath.Join(root, "logo.png"), 800, 600)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=logo.png&res=480p", nil)
	req.Header.Set("Accept", "image/png")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
}

func writeWebPFixture(t *testing.T, p *vipsproc.Pipeline, path string, w, h int) {
	// Go's stdlib has no WebP encoder; generate valid WebP bytes via libvips.
	data, _, err := p.ProcessImage(image.NewRGBA(image.Rect(0, 0, w, h)),
		vipsproc.Options{Format: vipsproc.FormatWebP})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func TestProcessDirectWebP(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writeWebPFixture(t, h.Vips, filepath.Join(root, "pic.webp"), 1000, 800)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=pic.webp&res=720p", nil)
	req.Header.Set("Accept", "image/webp")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/webp", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
}

func TestProcessDirectIgnoresRawParams(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writeJPEGFixture(t, filepath.Join(root, "photo.jpg"), 1600, 900)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=photo.jpg&res=720p&wb=camera&exp=1.5", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
}

func TestProcessDirectCorruptIsUnsupported(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	// Valid JPEG SOI+marker, intentionally truncated body: detected as JPEG,
	// relies on libvips rejecting it as undecodable.
	require.NoError(t, os.WriteFile(filepath.Join(root, "bad.jpg"),
		append([]byte{0xFF, 0xD8, 0xFF}, []byte("not a real jpeg")...), 0644))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=bad.jpg", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Contains(t, rec.Body.String(), "unsupported_format")
}
