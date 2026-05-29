# Design: Non-RAW input support (JPEG / PNG / WebP)

**Date:** 2026-05-29
**Status:** Approved (design), pending implementation plan

## Summary

`roxy` currently routes every `/process` request through LibRaw. Non-RAW input
(JPEG, PNG, etc.) fails because LibRaw can only decode files that carry sensor
mosaic data. This change lets `roxy` accept **JPEG, PNG, and WebP** source files
by detecting them up front and decoding them directly with `libvips`, bypassing
LibRaw entirely.

The rest of the stack already works regardless of input format: the local
backend serves any file by path, format negotiation / presets / cache key /
ETag / eviction are all format-agnostic, and `vipsproc` already loads arbitrary
encoded buffers via `vips.NewImageFromBuffer`. The only blocker is the routing
decision in `httpapi.process()`.

## Goals

- Accept JPEG, PNG, and WebP source files on `GET /process`.
- Reuse the existing resize/encode/cache/negotiation machinery unchanged.
- Preserve current RAW behavior exactly for everything that is not a recognized
  web format.

## Non-goals

- TIFF input. TIFF magic bytes (`II*\0` / `MM\0*`) collide with TIFF-based RAW
  (CR2, NEF, ARW, DNG), and LibRaw cannot reliably decode ordinary photographic
  TIFFs. Plain TIFFs therefore remain unsupported (best-effort via the LibRaw
  fall-through, which usually fails). Revisit later if needed.
- HEIF / AVIF input (would require confirming `libheif` in the libvips build).
- A megapixel / decode-bomb guard. `ROXY_MAX_INPUT_SIZE_MB` plus libvips
  shrink-on-load remain the only input limits. No new config.
- Any change to the RAW fast-path / slow-path logic.

## Key decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Format detection | Content sniff (magic bytes) | Robust against misnamed files; no extension table to maintain. |
| Detection scope | JPEG, PNG, WebP only; all else → RAW | These three magic signatures never collide with RAW. TIFF magic does, so TIFF is excluded. |
| RAW-only params on non-RAW input | Silently ignore | `wb`, `exp`, `half_size`, `embed_only` are meaningless for an already-rendered image; ignoring is lenient for callers sending a blanket param set. |
| Decode-bomb guard | None | Rely on existing size cap + shrink-on-load. |
| Cache key | Unchanged | See "Caching" below. |

## Architecture

### New package: `internal/inputfmt`

A small, dependency-free classifier with a single responsibility.

```go
package inputfmt

type Kind int

const (
    KindRAW  Kind = iota // default / fall-through — hand to LibRaw
    KindJPEG
    KindPNG
    KindWebP
)

// Detect classifies input from its leading bytes. It positively recognizes
// JPEG, PNG, and WebP; anything else (including TIFF and all RAW formats)
// returns KindRAW so the existing LibRaw path handles it.
//
// header should contain at least the first 12 bytes of the file; fewer is
// safe (returns KindRAW).
func Detect(header []byte) Kind
```

Detection rules (first match wins):

- JPEG: `header[0:3] == FF D8 FF`
- PNG: `header[0:8] == 89 50 4E 47 0D 0A 1A 0A`
- WebP: `header[0:4] == "RIFF"` and `header[8:12] == "WEBP"`
- otherwise: `KindRAW`

`Kind` exposes a helper for whether it is a direct-decode format:

```go
func (k Kind) Direct() bool { return k != KindRAW }
```

### `internal/vipsproc`: rename `ProcessJPEG` → `ProcessEncoded`

`ProcessJPEG` already does `NewImageFromBuffer` → `AutoRotate` → resize → encode,
which is format-agnostic. Rename it to `ProcessEncoded` to reflect that it now
serves JPEG, PNG, and WebP buffers as well as RAW embedded previews. Pure rename,
no behavior change. Two call sites: the RAW fast path and the new direct path.

### `internal/httpapi`: routing in `process()`

`process()` currently: materialize → (fast: extract preview → `ProcessJPEG`) →
(slow: demosaic → `ProcessImage`).

New flow:

1. `materialize(ctx, req.File)` → local path (unchanged).
2. Open the file, read the first 16 bytes, `kind := inputfmt.Detect(header)`.
3. **`kind.Direct()` (JPEG / PNG / WebP):**
   - `os.ReadFile(localPath)` → `Vips.ProcessEncoded(bytes, vipsOpts(req, format))`.
   - Return with `pathTaken = "direct"`.
   - No slow-path semaphore (no demosaicing → no DoS surface to gate).
   - `wb`, `exp`, `half_size`, `embed_only` are ignored (not read on this branch).
4. **`KindRAW`:** existing fast-path / slow-path logic, unchanged.

`vipsOpts` is reused as-is (it only depends on preset + format).

## Data flow

```
GET /process
  → ParseRequest / Negotiate / Stat / size check        (unchanged)
  → cache key + If-None-Match + cache lookup             (unchanged)
  → MISS → process():
        materialize → read 16-byte header → Detect
            ├── JPEG/PNG/WebP → ProcessEncoded → X-Path: direct
            └── RAW          → fast/slow path  → X-Path: fast|slow
  → respond + async cache store                          (unchanged)
```

## Error handling

- Direct-path libvips decode failure (truncated / corrupt image) →
  `ErrUnsupportedFormat` → HTTP **415**, consistent with how slow-path decode
  failures already map (`handler.go:248`).
- File read errors after materialize → `ErrInternal` → **500**, matching the
  existing materialize error mapping.
- A file whose magic is none of JPEG/PNG/WebP and which LibRaw also rejects
  (e.g. a `.txt` or video) → falls through to the RAW path and surfaces LibRaw's
  error as today (415 on the slow path). No new behavior.

## Caching

`cachekey.Compute` is unchanged; it already keys on backend, file, size, mtime,
res, format, wb, exp, half_size, embed_only.

Trade-off from "silently ignore RAW-only params": for a non-RAW file,
`?file=a.jpg&wb=camera` and `?file=a.jpg&wb=auto` produce different cache keys
but byte-identical output → a duplicate cache entry in that misuse case. This is
benign (correctness is unaffected; eviction reclaims it). Normalizing the key
would require sniffing the file *before* the cache lookup, opening the source on
every request including cache hits — which defeats the cache. Rejected.

## Response headers

- `X-Path: direct` is added as a third value alongside `cache`, `fast`, `slow`.
- All other headers (`X-Cache`, `ETag`, `Cache-Control`, `Vary`, `Content-Type`,
  `Content-Length`) are produced by existing code paths and need no change.

## Configuration

No new environment variables.

## Testing

- **`inputfmt`** — table-driven unit tests: each of JPEG/PNG/WebP magic →
  correct `Kind`; TIFF magic → `KindRAW`; truncated/short header → `KindRAW`;
  arbitrary bytes → `KindRAW`. `Direct()` helper.
- **`vipsproc`** — existing `ProcessJPEG` tests renamed; add a PNG and a WebP
  buffer case to confirm `ProcessEncoded` resizes/encodes them. Gated by the
  existing `vips` build tag.
- **`httpapi`** — handler tests with small JPEG/PNG/WebP fixtures:
  - 200, correct `Content-Type` (from `Accept` negotiation), `X-Path: direct`.
  - Ignored-param test: `?...&wb=camera&exp=1.5` on a JPEG still returns 200.
  - Corrupt-image fixture → 415.
- **Fixtures** — small JPEG/PNG/WebP test files under
  `internal/httpapi/testdata/` (and any needed by `inputfmt`).

## Documentation

Update `README.md`:

- Line 3 / line 15 / line 58: state that JPEG, PNG, and WebP are accepted as
  source input in addition to RAW.
- Line 137 / API table: note RAW-only params are ignored for non-RAW input.
- Path diagram + `X-Path` header docs: add the `direct` path.

## Affected files

| File | Change |
|------|--------|
| `internal/inputfmt/inputfmt.go` | New: `Kind`, `Detect`, `Direct`. |
| `internal/inputfmt/inputfmt_test.go` | New: detection table tests. |
| `internal/vipsproc/pipeline.go` | Rename `ProcessJPEG` → `ProcessEncoded`. |
| `internal/vipsproc/pipeline_test.go` | Rename + add PNG/WebP cases. |
| `internal/httpapi/handler.go` | Detect after materialize; route direct vs RAW; `ProcessEncoded` rename. |
| `internal/httpapi/handler_test.go` | New direct-path / ignored-param / corrupt cases. |
| `internal/httpapi/testdata/` | New JPEG/PNG/WebP fixtures. |
| `README.md` | Supported inputs, param semantics, path diagram. |
