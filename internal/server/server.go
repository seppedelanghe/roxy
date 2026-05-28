package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/seppedelanghe/roxy/internal/httpapi"
)

type Options struct {
	Port           int
	RequestTimeout time.Duration
	ShutdownGrace  time.Duration
	Handler        *httpapi.Handler
	CacheDir       string
	Log            *slog.Logger
	EvictRunner    func(ctx context.Context)
}

func Run(opts Options) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", opts.Handler.Healthz)
	mux.HandleFunc("/process", opts.Handler.Process)

	var draining atomic.Bool
	opts.Handler.Healthy = func() bool { return !draining.Load() }

	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(opts.Port)),
		Handler:           withTimeout(mux, opts.RequestTimeout),
		ReadHeaderTimeout: 5 * time.Second,
	}

	evictCtx, cancelEvict := context.WithCancel(context.Background())
	defer cancelEvict()
	if opts.EvictRunner != nil {
		go opts.EvictRunner(evictCtx)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-stop:
		opts.Log.Info("shutdown signal received, draining")
		draining.Store(true)
	}

	go func() {
		<-stop
		opts.Log.Warn("second signal, exiting now")
		os.Exit(1)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), opts.ShutdownGrace)
	defer cancel()
	err := srv.Shutdown(ctx)
	cancelEvict()
	sweepTmp(opts.CacheDir)
	return err
}

func withTimeout(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func sweepTmp(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
