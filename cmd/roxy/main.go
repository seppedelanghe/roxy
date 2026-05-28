package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/seppedelanghe/roxy/internal/backend"
	"github.com/seppedelanghe/roxy/internal/config"
	"github.com/seppedelanghe/roxy/internal/diskcache"
	"github.com/seppedelanghe/roxy/internal/httpapi"
	"github.com/seppedelanghe/roxy/internal/logging"
	"github.com/seppedelanghe/roxy/internal/raw"
	"github.com/seppedelanghe/roxy/internal/server"
	"github.com/seppedelanghe/roxy/internal/vipsproc"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	log := logging.New(cfg.LogLevel)

	be, err := backend.NewLocal(cfg.SourceDir)
	if err != nil {
		log.Error("backend init", "err", err)
		os.Exit(2)
	}

	lru := diskcache.NewLRU(0)
	if err := diskcache.Sweep(cfg.CacheDir, lru); err != nil {
		log.Error("cache sweep", "err", err)
		os.Exit(2)
	}
	cache := diskcache.New(cfg.CacheDir, lru)

	vp := vipsproc.NewPipeline()

	h := &httpapi.Handler{
		Backend:      be,
		Cache:        cache,
		RAW:          raw.NewAdapter(),
		Vips:         vp,
		Log:          log,
		MaxInputSize: cfg.MaxInputSizeMB * 1024 * 1024,
		CacheMaxAge:  cfg.CacheControlMaxAge,
		SlowGate:     make(chan struct{}, cfg.MaxConcurrentDemosaic),
		SlowWait:     cfg.SlowPathWait,
	}

	log.Info("roxy starting", "port", cfg.Port, "backend", be.ID())
	err = server.Run(server.Options{
		Port:           cfg.Port,
		RequestTimeout: cfg.RequestTimeout,
		ShutdownGrace:  cfg.ShutdownGrace,
		Handler:        h,
		CacheDir:       cfg.CacheDir,
		Log:            log,
		EvictRunner: func(ctx context.Context) {
			diskcache.Run(ctx, cache,
				cfg.CacheMaxSizeGB*1024*1024*1024,
				time.Duration(cfg.CacheTTLDays)*24*time.Hour,
				1*time.Minute,
			)
		},
	})
	if err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}
