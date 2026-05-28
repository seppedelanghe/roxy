# E2E Verification — 2026-05-28

Manual verification of roxy v1 against a real Nikon RAW (`_SPC5194.NEF`, 75 MB).

Run with:

    ROXY_SOURCE_DIR=$(pwd)/testdata/source \
    ROXY_CACHE_DIR=$(pwd)/testdata/cache \
    ./bin/roxy

| # | Scenario | Result | Notes |
|---|----------|--------|-------|
| 1 | `GET /healthz` | 200 `ok` | |
| 2 | Fast path JPEG 720p, cache miss | 200, `X-Path: fast`, `X-Cache: MISS`, 58 KB JPEG, 1079×720 | 189 ms end-to-end |
| 3 | Repeat request — cache hit | 200, `X-Path: cache`, `X-Cache: HIT` | sub-ms |
| 4 | `If-None-Match` matches ETag | 304, body empty | ETag is `"<sha256>"` (quoted strong validator) |
| 5 | `Accept: image/heif` | 415 `unsupported_format` | JSON error envelope |
| 6 | `res=4k` (not whitelisted) | 400 `bad_request` | |
| 7 | `file=../etc/passwd` | 400 `invalid_file_key` | path traversal rejected |
| 8 | `file=nope.NEF` | 404 `not_found` | |
| 9 | Slow path `wb=camera` 480p WebP | 200, WebP 719×480, 9.1 s | full demosaic — expected order of magnitude |
| 10 | `SIGTERM` → graceful shutdown | clean exit, "draining" logged | |

Skipped (would require additional assets):
- Source replacement → key change (works by spec — `Stat` is called every request).
- 503 `at_capacity` under saturation (not exercised; semaphore behavior covered by unit code).
- `embed_only=true` against a preview-less RAW (no such RAW available locally).

Build environment: macOS 14, Go 1.26.3, libraw 0.21.x (Homebrew), libvips 8.18.2.
