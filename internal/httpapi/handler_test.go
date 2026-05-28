package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/seppedelanghe/roxy/internal/backend"
	"github.com/seppedelanghe/roxy/internal/diskcache"
	"github.com/seppedelanghe/roxy/internal/logging"
)

func newTestHandler(t *testing.T) (*Handler, string) {
	root := t.TempDir()
	cacheDir := t.TempDir()
	b, err := backend.NewLocal(root)
	require.NoError(t, err)
	lru := diskcache.NewLRU(0)
	cache := diskcache.New(cacheDir, lru)
	h := &Handler{
		Backend:      b,
		Cache:        cache,
		Log:          logging.NewTo(os.Stderr, "error"),
		MaxInputSize: 100 << 20,
		CacheMaxAge:  60,
		SlowGate:     make(chan struct{}, 1),
		SlowWait:     0,
		Healthy:      func() bool { return true },
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
