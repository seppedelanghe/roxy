# Specification: `roxy` (RAW-First Image Optimization Proxy)

`roxy` is a high-performance, open-source microservice written in Go for on-the-fly processing, resizing, and caching of camera RAW images (`.CR2`, `.CR3`, `.NEF`, `.ARW`, `.DNG`, etc.).

It pairs custom Go LibRaw bindings with `libvips` to deliver an open-source alternative to commercial image proxies, optimizing RAW processing speed via an embedded-preview extraction shortcut.

---

## 1. Core Architecture & Request Lifecycle

To maintain high throughput and low CPU usage, `roxy` splits the pipeline into a **Fast Path** (embedded preview extraction) and a **Slow Path** (full sensor demosaicing).

### Fast-Path Eligibility

A RAW file qualifies for the Fast Path only when **all** of the following hold:

1. LibRaw reports at least one embedded JPEG preview.
2. The largest embedded preview's longest edge is **≥ the longest edge of the requested resolution** (i.e., the resize will be a downscale, not an upscale). If the largest preview is smaller (e.g., the 160×120 thumbnails some older cameras embed) the Fast Path is skipped and the request falls through to the Slow Path — or, if `embed_only=true`, returns `422 no_embedded_preview`.
3. No RAW-adjustment parameter is set (`wb=auto`, `exp=0`).

### Response Buffering

Output images from the libvips pipeline are encoded into an in-memory byte buffer **before** the HTTP response begins. This lets `roxy`:

- Set an accurate `Content-Length` header.
- Compute the `ETag` (already known from the cache key) and write headers in a single pass.
- Stream the buffered bytes to the client without chunked transfer encoding.

After the response is flushed, the same buffer is handed to a background goroutine that writes it to disk via the atomic `<key>.tmp` → rename dance (see §4). A failed cache write does not affect the response that was already sent; the next request will simply re-render and retry. Since downscaled outputs (1080p WebP/JPEG) are typically 200 KB–1.5 MB, the memory cost of buffering is bounded and trivial relative to the RAW decode itself.

### Orientation Normalization

Embedded JPEGs typically carry an EXIF `Orientation` tag rather than physically rotated pixels; demosaiced output is auto-rotated by LibRaw into the correct pixel orientation with no EXIF tag. To guarantee both paths return visually identical pixel arrays, the libvips pipeline calls `vips_autorot()` (or equivalent) as its first step on Fast-Path input and strips the orientation tag. Slow-Path input is configured to auto-rotate at the LibRaw layer. Output images carry no `Orientation` EXIF tag — all rotation is baked into pixels.

```
                 [ Incoming HTTP GET Request ]
                              │
                              ▼
                 ┌──────────────────────────┐
                 │  Cache Check (Key/HMAC)  │
                 └────────────┬─────────────┘
                              │
                      ┌───────┴───────┐
                Cache Hit?        Cache Miss?
                    │                 │
                    ▼                 ▼
          [ Serve Cached File ]  [ Open RAW via Storage Backend ]
                                      │
                                      ▼
                            Does large embedded
                           JPEG preview exist?
                           /                 \
                        YES                  NO
                        /                     \
         ┌─────────────────────────┐   ┌──────────────────────────┐
         │     FAST PATH           │   │      SLOW PATH           │
         │ Extract Embedded JPEG   │   │ Full LibRaw Demosaicing  │
         │ Execution: ~1-5ms       │   │ Execution: ~100-300ms    │
         └────────────┬────────────┘   └────────────┬─────────────┘
                      │                             │
                      └──────────────┬──────────────┘
                                     │ (Pass raw byte array)
                                     ▼
                        ┌──────────────────────────┐
                        │   libvips Pipeline       │
                        │ (Resize, Crop, Format)   │
                        └────────────┬─────────────┘
                                     │
                                     ▼
                        ┌──────────────────────────┐
                        │ Buffer → Respond → Cache │
                        └──────────────────────────┘
```

---

## 2. API Endpoints

### `GET /process`

Fetches, processes, and returns an image according to the specified modifiers. The output format is selected via the `Accept` request header (see §3).

* **Example (standard):**
  `GET /process?file=gallery/sunset.NEF&res=720p`
  `Accept: image/webp`
* **Example (RAW adjustment, forces slow path):**
  `GET /process?file=studio/model.CR3&res=1080p&wb=camera&exp=1.5`
  `Accept: image/jpeg`

### `GET /healthz`

Liveness/readiness probe for container orchestration. Returns `200 OK` with body `ok`.

### Response Headers (all successful image responses)

| Header | Value |
| --- | --- |
| `Content-Type` | Negotiated output type: `image/jpeg`, `image/png`, or `image/webp` |
| `Content-Length` | Byte length of the body |
| `Cache-Control` | `public, max-age=<ROXY_CACHE_CONTROL_MAX_AGE>` |
| `ETag` | Strong validator — the cache key (see §4) |
| `Vary` | `Accept` (the response body depends on content negotiation) |
| `X-Cache` | `HIT` or `MISS` |
| `X-Path` | `cache`, `fast`, or `slow` (which pipeline served the request) |

### Conditional Requests

If the client sends `If-None-Match` matching the current `ETag`, `roxy` responds `304 Not Modified` with no body and the same caching headers. `If-Modified-Since` is **not** honored (the cache key is the only source of truth for content identity).

---

## 3. Query Parameters & Content Negotiation

| Parameter | Type | Default | In cache key? | Description |
| --- | --- | --- | --- | --- |
| `file` | `string` | *required* | yes | Backend-relative object key. Path traversal (`..`) and absolute paths are rejected. |
| `res` | `string` | `source` | yes | Resolution preset (`thumb`, `480p`, `720p`, `1080p`). See §5 for the full list. Arbitrary `WxH` values are rejected. |
| `embed_only` | `bool` | `false` | yes | If `true`, returns `422` when no embedded preview is present (prevents slow-path CPU spikes). |
| `wb` | `string` | `auto` | yes | White balance: `auto`, `camera`, or `daylight`. Any non-default value forces slow path. |
| `exp` | `float` | `0.0` | yes | Exposure correction in stops (`-3.0` to `3.0`). Any non-zero value forces slow path. |
| `half_size` | `bool` | `false` | yes | Tells LibRaw to read raw data at half-size — fast fallback when slow path is unavoidable. |

### Output Format (Accept header)

The response format is negotiated via the `Accept` request header. Supported media types: `image/jpeg`, `image/png`, `image/webp`. The negotiated type is part of the cache key (see §4) so each format is cached independently.

- If `Accept` is missing or `*/*`, the server defaults to `image/jpeg`.
- If `Accept` lists multiple supported types with quality values, the highest-q supported type wins.
- If `Accept` lists only unsupported types, the server responds `415 Unsupported Media Type`.

### Validation Rules

- Unknown query parameters → `400 Bad Request`.
- Out-of-range numeric values → `400 Bad Request`.
- Any RAW-adjustment parameter (`wb≠auto`, `exp≠0`) automatically disables the fast path even if a preview exists.
- Source files larger than `ROXY_MAX_INPUT_SIZE_MB` are rejected with `413 Payload Too Large`.

---

## 4. Caching Strategy

A two-tiered cache minimizes disk I/O and redundant CPU cycles.

### Cache Key

Generated deterministically from the normalized cache-key parameters, the negotiated output format, the configured backend identifier, and a **source fingerprint** that pins the key to a specific version of the underlying file:

```
sha256(
    backend_id + "|" + file + "|" +
    source_size + "|" + source_mtime_unix_nano + "|" +
    res + "|" + format + "|" + wb + "|" + exp + "|" + half_size + "|" + embed_only
)
```

`format` is the resolved output media type (`jpeg`, `png`, or `webp`) from `Accept` negotiation, not a raw header value.

`source_size` and `source_mtime_unix_nano` come from a `Backend.Stat()` call performed at the start of every request (before the cache lookup). If the source file is replaced — same key, different bytes — the fingerprint changes, the cache key changes, and the stale entry is naturally orphaned (eventually reclaimed by LRU/TTL eviction). The stat call adds one syscall per request on the local backend and one `HEAD` per request on future S3 backends; both are cheap relative to the rest of the pipeline.

The hex-encoded digest is used as both the on-disk cache filename and the `ETag` header.

### Tier 1: In-Process Metadata

An in-memory LRU of recent cache-key → file metadata (size, content-type, mtime) avoids a `stat()` syscall on hot keys.

### Tier 2: Disk Persistence

- Files are written to `ROXY_CACHE_DIR` (e.g., `/var/cache/roxy/`).
- Atomic writes: write to `<key>.tmp`, then rename.
- **LRU eviction:** a background ticker tracks total cache size; when usage exceeds `ROXY_CACHE_MAX_SIZE_GB`, least-recently-used entries are evicted until usage drops back below the cap (with a small headroom to avoid thrashing).
- TTL: entries older than `ROXY_CACHE_TTL_DAYS` are eligible for early eviction regardless of recency.

---

## 5. Security & Protection (Anti-DDoS)

Because processing 50MB+ RAW files is resource-intensive, `roxy` applies guardrails.

### Resolution Whitelisting

Only preset resolutions are accepted. Arbitrary `WxH` values are rejected with `400 Bad Request`. The engine maps requests to an internal preset configuration:

```json
{
  "presets": {
    "thumb":  {"width": 150,  "height": 150,  "fit": "crop"},
    "480p":   {"width": 854,  "height": 480,  "fit": "scale"},
    "720p":   {"width": 1280, "height": 720,  "fit": "scale"},
    "1080p":  {"width": 1920, "height": 1080, "fit": "scale"}
  }
}
```

### Additional Guardrails

- **Max input size:** `ROXY_MAX_INPUT_SIZE_MB` (default `100`).
- **Request timeout:** `ROXY_REQUEST_TIMEOUT_SECONDS` (default `30`) — per-request deadline propagated to backend reads and decoding.
- **Concurrency:** see §7.

---

## 6. Storage Backends

Source RAW files are fetched through a pluggable `Backend` interface. The v1 release ships **local filesystem** only. The interface is the seam where S3-compatible backends (MinIO, R2, AWS) will plug in later — no client code is included in v1.

```go
type Stat struct {
    Size    int64
    ModTime time.Time
}

// Object is a random-access view of a backend object. Implementations
// must support concurrent ReadAt calls so LibRaw can seek without
// buffering the full file in memory.
type Object interface {
    io.ReaderAt
    io.Closer
}

type Backend interface {
    // Open returns a random-access handle to the object at key.
    // Returns ErrNotFound if the object does not exist.
    Open(ctx context.Context, key string) (Object, Stat, error)

    // Stat returns metadata without opening the body.
    Stat(ctx context.Context, key string) (Stat, error)

    // ID returns a stable identifier for this backend instance,
    // included in cache keys so a key collision across backends is impossible.
    ID() string
}
```

The `ReaderAt` shape is chosen so future remote backends (S3, R2) can implement `Open` with HTTP range requests rather than streaming the full object into memory. The local backend wraps `*os.File`, which already satisfies the interface.

### Note for Future Remote Backends

LibRaw performs many small, non-sequential reads when parsing a RAW file: it skips between headers, metadata blocks at the end of the file, and embedded preview offsets. A naive S3/R2 implementation that translates each `ReadAt` into a single HTTP `Range` request will issue dozens of round-trips per decode and push decode latency into the multi-second range.

Remote backends MUST do one of the following:

- **Chunked block cache wrapper:** wrap the remote object with a read-through cache that fetches in fixed-size blocks (e.g., 1–4 MiB) and coalesces small `ReadAt` calls into a small number of range requests.
- **Scratch-to-disk for the slow path:** when a request is dispatched to the Slow Path, download the full object to a per-request scratch file in a local tmpfs (e.g., `/tmp/roxy-scratch/`) and hand LibRaw a local `*os.File`. The scratch file is deleted when the request completes.

The Fast Path is far less affected because embedded JPEG extraction touches only a few well-defined byte ranges; a single ranged read for the metadata block followed by one for the preview payload is usually sufficient.

Selection is driven by `ROXY_SOURCE_BACKEND`:

- `local` (default) — rooted at `ROXY_SOURCE_DIR`. Keys are joined with the root and the result must remain inside the root (path-traversal rejected). Symlinks are not followed across the root boundary.

Future backends (`s3`, etc.) will be added behind the same interface; no API change is anticipated.

---

## 7. Concurrency & Backpressure

- The slow path is gated by a counting semaphore of size `ROXY_MAX_CONCURRENT_DEMOSAIC` (default `4`).
- When the semaphore is exhausted, a slow-path request will wait up to `ROXY_SLOW_PATH_WAIT_MS` (default `500`) for a slot to free up. Since a typical slow-path execution takes 100–300ms, this brief wait often clears without an extra round-trip.
- If no slot becomes available within the wait window, the request is rejected with `503 Service Unavailable` and a `Retry-After` header containing a small randomized value in seconds (jittered in `[1, 5]`) to prevent thundering-herd retries.
- The fast path is not semaphore-gated — it is cheap and bounded by the HTTP server's connection limit.
- Request coalescing / single-flight for identical cache keys is **out of scope for v1**.

---

## 8. Error Model

All error responses use this JSON body:

```json
{ "error": "human-readable message", "code": "snake_case_identifier" }
```

| Status | `code` | When |
| --- | --- | --- |
| `400` | `bad_request` | Unknown param, malformed value, invalid or non-whitelisted `res`. |
| `400` | `invalid_file_key` | Path traversal or absolute path in `file`. |
| `404` | `not_found` | Source key missing in backend. |
| `413` | `input_too_large` | Source exceeds `ROXY_MAX_INPUT_SIZE_MB`. |
| `415` | `unsupported_format` | `Accept` lists no supported media type, or input is an unrecognized RAW. |
| `422` | `no_embedded_preview` | `embed_only=true` and the RAW has no embedded preview. |
| `503` | `at_capacity` | Slow-path semaphore exhausted after the bounded wait (see §7). |
| `500` | `internal_error` | Unexpected processing failure. |

Successful responses (`200`, `304`) never carry a JSON envelope — the body is the image (or empty for `304`).

---

## 9. Logging

`roxy` writes structured JSON logs to stdout, one line per request, plus startup/shutdown events.

### Per-request fields

| Field | Type | Description |
| --- | --- | --- |
| `ts` | string (RFC3339Nano) | Timestamp |
| `level` | string | `info`, `warn`, `error` |
| `method` | string | HTTP method |
| `path` | string | URL path |
| `query` | string | Raw query string |
| `status` | int | HTTP status |
| `duration_ms` | int | Total request duration |
| `path_taken` | string | `cache`, `fast`, or `slow` |
| `cache` | string | `hit` or `miss` |
| `bytes_out` | int | Response body bytes |
| `backend` | string | Backend `ID()` |
| `error` | string | Error message (only when `status >= 400`) |
| `error_code` | string | Error `code` (only when `status >= 400`) |

Log verbosity is controlled by `ROXY_LOG_LEVEL` (`debug`, `info`, `warn`, `error`; default `info`).

Metrics, tracing, and access-log-style multi-line records are explicitly out of scope for v1.

---

## 10. Environment Configuration

The microservice is configured entirely via environment variables.

```bash
# Server
ROXY_PORT=8080
ROXY_REQUEST_TIMEOUT_SECONDS=30
ROXY_LOG_LEVEL=info
ROXY_SHUTDOWN_GRACE_SECONDS=30    # see §12

# Source storage (pluggable backend)
ROXY_SOURCE_BACKEND=local         # v1: local only; future: s3
ROXY_SOURCE_DIR=/mnt/raw_storage  # required when backend=local
ROXY_MAX_INPUT_SIZE_MB=100

# Cache
ROXY_CACHE_DIR=/mnt/fast_cache
ROXY_CACHE_MAX_SIZE_GB=50         # LRU eviction triggers when exceeded
ROXY_CACHE_TTL_DAYS=7
ROXY_CACHE_CONTROL_MAX_AGE=86400  # seconds, sent as Cache-Control to clients

# Concurrency
ROXY_MAX_CONCURRENT_DEMOSAIC=4    # slow-path semaphore size
ROXY_SLOW_PATH_WAIT_MS=500        # wait window before 503 (see §7)
```

---

## 11. Technical Dependencies & Compilation

### Go modules

| Module | Minimum version | Purpose |
| --- | --- | --- |
| `github.com/seppedelanghe/go-libraw` | latest | LibRaw bindings (self-authored) |
| `github.com/davidbyttow/govips/v2` | `>= 2.13.x` | libvips bindings |

### System libraries

| Library | Minimum version | Notes |
| --- | --- | --- |
| Go toolchain | `>= 1.24` | Build-time |
| LibRaw | `>= 0.21.x` | C dev headers at build time; `libraw23` runtime |
| libvips | `>= 8.14.x` | C dev headers at build time; `libvips42` runtime |

### Docker Multi-Stage Build (blueprint)

```dockerfile
# Stage 1: build with C dependencies
FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y \
    libraw-dev \
    libvips-dev \
    build-essential

WORKDIR /app
COPY . .
RUN go build -tags vips -o roxy ./cmd/roxy

# Stage 2: minimal runtime image
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y libraw23 libvips42 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/roxy /usr/local/bin/roxy
EXPOSE 8080
ENTRYPOINT ["roxy"]
```

---

## 12. Startup & Shutdown

### Startup: Cache Sweep

On boot — before the HTTP listener is accepted — `roxy` performs a one-shot scan of `ROXY_CACHE_DIR`:

- Any file matching `*.tmp` is a partial write left by a crashed previous run and is deleted.
- The in-memory LRU metadata tier (see §4) is rebuilt from the surviving files' `stat()` results so eviction state is consistent from the first request.
- If the cache directory does not exist, it is created (mode `0755`); fatal startup error if creation fails.

**Note on LRU ordering after restart:** the rebuild uses `mtime` as a proxy for last-access time, since most Linux filesystems are mounted with `noatime`/`relatime` and atime is unreliable. This means the cache behaves more like FIFO immediately after a restart and gradually re-acquires true LRU ordering as live traffic touches entries and updates their in-memory access timestamps. This is acceptable for v1 — the operational impact is a slightly suboptimal eviction order for the first few minutes of post-restart traffic.

The sweep is synchronous so the process is never serving traffic with an inconsistent cache view.

### Graceful Shutdown

On `SIGINT` or `SIGTERM`, `roxy` enters drain mode:

1. The HTTP server stops accepting new connections immediately. `/healthz` begins returning `503` so orchestrators stop routing traffic.
2. In-flight requests are allowed to complete, bounded by `ROXY_SHUTDOWN_GRACE_SECONDS` (default `30`). Their request contexts are **not** canceled during the grace window — a slow-path demosaic that started just before SIGTERM gets to finish.
3. If the grace window elapses, remaining request contexts are canceled and the server exits.
4. Background workers (cache eviction ticker) stop on the same signal; any in-progress eviction pass is allowed to finish its current file rename before exiting.
5. The final shutdown step removes leftover `*.tmp` files in `ROXY_CACHE_DIR` (a best-effort mirror of the startup sweep).

A second `SIGINT`/`SIGTERM` during drain triggers immediate exit with no grace period.
