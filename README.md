# roxy

RAW-first image optimization proxy. See `spec.md` for the full specification and `docs/superpowers/plans/` for implementation history.

## Build

Requires: Go 1.24+, libraw-dev, libvips-dev.

    make build

## Run

See `spec.md` §10 for environment variables. Minimal:

    ROXY_SOURCE_DIR=/path/to/raw ROXY_CACHE_DIR=/tmp/roxy-cache ./bin/roxy
