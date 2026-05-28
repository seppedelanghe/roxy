package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ROXY_SOURCE_DIR", "/tmp/src")
	t.Setenv("ROXY_CACHE_DIR", "/tmp/cache")
	cfg, err := Load()
	require.NoError(t, err)
	require.Equal(t, 8080, cfg.Port)
	require.Equal(t, 30*time.Second, cfg.RequestTimeout)
	require.Equal(t, "info", cfg.LogLevel)
	require.Equal(t, "local", cfg.SourceBackend)
	require.Equal(t, "/tmp/src", cfg.SourceDir)
	require.Equal(t, int64(100), cfg.MaxInputSizeMB)
	require.Equal(t, "/tmp/cache", cfg.CacheDir)
	require.Equal(t, int64(50), cfg.CacheMaxSizeGB)
	require.Equal(t, 7, cfg.CacheTTLDays)
	require.Equal(t, 86400, cfg.CacheControlMaxAge)
	require.Equal(t, 4, cfg.MaxConcurrentDemosaic)
	require.Equal(t, 500*time.Millisecond, cfg.SlowPathWait)
	require.Equal(t, 30*time.Second, cfg.ShutdownGrace)
}

func TestLoadRequiresSourceDirForLocal(t *testing.T) {
	t.Setenv("ROXY_SOURCE_BACKEND", "local")
	t.Setenv("ROXY_SOURCE_DIR", "")
	t.Setenv("ROXY_CACHE_DIR", "/tmp/cache")
	_, err := Load()
	require.Error(t, err)
}

func TestLoadRejectsUnknownBackend(t *testing.T) {
	t.Setenv("ROXY_SOURCE_BACKEND", "s3")
	t.Setenv("ROXY_SOURCE_DIR", "/tmp/src")
	t.Setenv("ROXY_CACHE_DIR", "/tmp/cache")
	_, err := Load()
	require.Error(t, err)
}

func TestLoadInvalidInt(t *testing.T) {
	t.Setenv("ROXY_PORT", "not-a-number")
	t.Setenv("ROXY_SOURCE_DIR", "/tmp/src")
	t.Setenv("ROXY_CACHE_DIR", "/tmp/cache")
	_, err := Load()
	require.Error(t, err)
}
