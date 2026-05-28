# roxy

**RAW-first image optimization proxy** on-the-fly processing, resizing, and caching of camera RAW files (`.CR2`, `.CR3`, `.NEF`, `.ARW`, `.DNG`, and more), served over HTTP.

![Go 1.24+](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go)

---

## What is roxy?

Most image proxies assume JPEG or PNG as input or forces a subscription for RAW images. `roxy` starts from RAW, the unprocessed sensor output that photographers and imaging pipelines work with, and delivers resized, web-ready images on demand.

It pairs my custom [Go LibRaw bindings](https://github.com/seppedelanghe/go-libraw) with `libvips` to deliver a fast, open-source alternative to commercial RAW image services.

**Supported RAW formats:** Canon CR2/CR3, Nikon NEF, Sony ARW, Adobe DNG, and all other formats supported by LibRaw 0.21+.

### How it stays fast

`roxy` splits every request into one of two paths:

| Path | Trigger | Typical latency |
|------|---------|-----------------|
| **Fast path** | RAW has a large enough embedded JPEG preview | ~1–5 ms |
| **Slow path** | Full LibRaw demosaicing required | ~100–300 ms |

The fast path extracts the embedded preview and hands it directly to the `libvips` resize pipeline, skipping demosaicing entirely. 
The slow path is gated by a concurrency semaphore so a burst of RAW-adjustment requests can't saturate the server.

```
[ GET /process ]
       │
       ▼
 Cache check ──── HIT ──→ serve cached file
       │
      MISS
       │
       ▼
  Open RAW via storage backend
       │
       ├── embedded JPEG large enough? ──YES──→ [ FAST PATH ] extract preview (~1–5ms)
       │                                                 │
       └── NO ───────────────────────────────→ [ SLOW PATH ] demosaic (~100–300ms)
                                                          │
                                                          ▼
                                                  libvips pipeline
                                               (resize, crop, format)
                                                          │
                                                          ▼
                                              buffer → respond → cache
```

---

## What roxy is not for

- **Arbitrary dimensions** - only whitelisted presets (`thumb`, `480p`, `720p`, `1080p`) are accepted. This is intentional: arbitrary `WxH` values are a DDoS vector for RAW processing.
- **RAW editing** - `roxy` applies basic white balance and exposure correction, but it is not a replacement for Lightroom, Darktable, or any full RAW editor.
- **Non-RAW input** - JPEG, PNG, TIFF, and video files are not (yet) supported as source material.
- **S3 / remote storage** - the v1 release ships with a local filesystem backend only. S3-compatible backends are on the roadmap but not included.
- **Metrics and tracing** - structured JSON logs are written to stdout, metrics and distributed tracing are out of scope.

---

## Getting started

### Prerequisites

- Go 1.24+ (build only)
- `libraw-dev` >= 0.21
- `libvips-dev` >= 8.14

### Build from source

```bash
make build
# binary is written to ./bin/roxy
```

### Run with Docker

```dockerfile
# Stage 1: build
FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y \
    libraw-dev libvips-dev build-essential

WORKDIR /app
COPY . .
RUN go build -tags vips -o roxy ./cmd/roxy

# Stage 2: minimal runtime
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y libraw23 libvips42 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/roxy /usr/local/bin/roxy
EXPOSE 8080
ENTRYPOINT ["roxy"]
```

```bash
docker build -t roxy .
docker run -p 8080:8080 \
  -e ROXY_SOURCE_DIR=/mnt/raw \
  -e ROXY_CACHE_DIR=/mnt/cache \
  -v /your/raw/files:/mnt/raw \
  -v /your/cache:/mnt/cache \
  roxy
```

### Minimal local run

```bash
ROXY_SOURCE_DIR=/path/to/raw/files \
ROXY_CACHE_DIR=/tmp/roxy-cache \
./bin/roxy
```

---

## API at a glance

### `GET /process`

Fetch, process, and return a resized image from a RAW source file.

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `file` | yes | / | Path to the RAW file relative to the storage root |
| `res` | no | `source` | Resolution preset: `thumb`, `480p`, `720p`, `1080p` |
| `wb` | no | `auto` | White balance: `auto`, `camera`, `daylight` |
| `exp` | no | `0.0` | Exposure correction in stops (`-3.0` to `3.0`) |
| `embed_only` | no | `false` | Return `422` instead of slow-path if no embedded preview |
| `half_size` | no | `false` | Half-size LibRaw decode (faster fallback for slow path) |

The output format is negotiated via the `Accept` header. Supported types: `image/jpeg`, `image/png`, `image/webp`. Defaults to `image/jpeg`.

**Examples:**

```bash
# Resize a NEF to 720p WebP (fast path if preview exists)
curl -H "Accept: image/webp" \
  "http://localhost:8080/process?file=gallery/sunset.NEF&res=720p" \
  -o sunset.webp

# Custom white balance + exposure -> forces slow path
curl -H "Accept: image/jpeg" \
  "http://localhost:8080/process?file=studio/model.CR3&res=1080p&wb=camera&exp=1.5" \
  -o model.jpg
```

**Key response headers:**

| Header | Description |
|--------|-------------|
| `X-Cache` | `HIT` or `MISS` |
| `X-Path` | `cache`, `fast`, or `slow` -> which pipeline served the request |
| `ETag` | Strong cache validator (SHA-256 of content key) |

### `GET /healthz`

Liveness probe. Returns `200 OK` with body `ok`.

---

## Configuration reference

All configuration is via environment variables.

```bash
# Server
ROXY_PORT=8080                       # default: 8080
ROXY_REQUEST_TIMEOUT_SECONDS=30      # per-request deadline
ROXY_SHUTDOWN_GRACE_SECONDS=30       # drain window on SIGTERM
ROXY_LOG_LEVEL=info                  # debug | info | warn | error

# Source storage
ROXY_SOURCE_BACKEND=local            # v1: local only
ROXY_SOURCE_DIR=/mnt/raw_storage     # required when backend=local
ROXY_MAX_INPUT_SIZE_MB=100           # reject files larger than this

# Cache
ROXY_CACHE_DIR=/mnt/fast_cache       # where processed images are stored
ROXY_CACHE_MAX_SIZE_GB=50            # LRU eviction triggers when exceeded
ROXY_CACHE_TTL_DAYS=7                # max age before eviction eligibility
ROXY_CACHE_CONTROL_MAX_AGE=86400     # Cache-Control max-age sent to clients (seconds)

# Concurrency
ROXY_MAX_CONCURRENT_DEMOSAIC=4       # slow-path semaphore size
ROXY_SLOW_PATH_WAIT_MS=500           # wait before returning 503
```

---

## Disclaimer

Parts of this project were developed with the assistance of large language models (LLMs). All generated code, configuration, and documentation has been reviewed, tested, and validated by the author. No AI-generated content has been shipped without human sign-off.
