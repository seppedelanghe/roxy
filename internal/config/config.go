package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port                  int
	RequestTimeout        time.Duration
	LogLevel              string
	ShutdownGrace         time.Duration

	SourceBackend  string
	SourceDir      string
	MaxInputSizeMB int64

	CacheDir           string
	CacheMaxSizeGB     int64
	CacheTTLDays       int
	CacheControlMaxAge int

	MaxConcurrentDemosaic int
	SlowPathWait          time.Duration
}

func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	if cfg.Port, err = envInt("ROXY_PORT", 8080); err != nil {
		return nil, err
	}
	secs, err := envInt("ROXY_REQUEST_TIMEOUT_SECONDS", 30)
	if err != nil {
		return nil, err
	}
	cfg.RequestTimeout = time.Duration(secs) * time.Second

	cfg.LogLevel = envStr("ROXY_LOG_LEVEL", "info")

	graceSecs, err := envInt("ROXY_SHUTDOWN_GRACE_SECONDS", 30)
	if err != nil {
		return nil, err
	}
	cfg.ShutdownGrace = time.Duration(graceSecs) * time.Second

	cfg.SourceBackend = envStr("ROXY_SOURCE_BACKEND", "local")
	if cfg.SourceBackend != "local" {
		return nil, fmt.Errorf("unsupported ROXY_SOURCE_BACKEND %q (v1 only supports \"local\")", cfg.SourceBackend)
	}
	cfg.SourceDir = envStr("ROXY_SOURCE_DIR", "")
	if cfg.SourceBackend == "local" && cfg.SourceDir == "" {
		return nil, errors.New("ROXY_SOURCE_DIR is required when backend=local")
	}

	maxIn, err := envInt("ROXY_MAX_INPUT_SIZE_MB", 100)
	if err != nil {
		return nil, err
	}
	cfg.MaxInputSizeMB = int64(maxIn)

	cfg.CacheDir = envStr("ROXY_CACHE_DIR", "")
	if cfg.CacheDir == "" {
		return nil, errors.New("ROXY_CACHE_DIR is required")
	}
	maxSize, err := envInt("ROXY_CACHE_MAX_SIZE_GB", 50)
	if err != nil {
		return nil, err
	}
	cfg.CacheMaxSizeGB = int64(maxSize)
	if cfg.CacheTTLDays, err = envInt("ROXY_CACHE_TTL_DAYS", 7); err != nil {
		return nil, err
	}
	if cfg.CacheControlMaxAge, err = envInt("ROXY_CACHE_CONTROL_MAX_AGE", 86400); err != nil {
		return nil, err
	}

	if cfg.MaxConcurrentDemosaic, err = envInt("ROXY_MAX_CONCURRENT_DEMOSAIC", 4); err != nil {
		return nil, err
	}
	waitMs, err := envInt("ROXY_SLOW_PATH_WAIT_MS", 500)
	if err != nil {
		return nil, err
	}
	cfg.SlowPathWait = time.Duration(waitMs) * time.Millisecond

	return cfg, nil
}

func envStr(k, def string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return def
}

func envInt(k string, def int) (int, error) {
	v, ok := os.LookupEnv(k)
	if !ok || v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q", k, v)
	}
	return n, nil
}
