# Non-RAW Input Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let `roxy` accept JPEG, PNG, and WebP source files by detecting them via magic bytes and decoding them directly with libvips, bypassing LibRaw.

**Architecture:** A new `internal/inputfmt` package classifies input from its leading bytes. `httpapi.process()` materializes the source as today, reads its first 16 bytes, and routes recognized web formats (JPEG/PNG/WebP) straight to the libvips pipeline (`X-Path: direct`); everything else falls through to the existing RAW fast/slow path unchanged. RAW-only params (`wb`, `exp`, `half_size`, `embed_only`) are silently ignored on the direct path.

**Tech Stack:** Go 1.24, govips/v2 (libvips), testify. Tests require libvips installed locally (no build tags in this repo). Reference spec: `docs/superpowers/specs/2026-05-29-non-raw-input-design.md`.

---

### Task 1: `internal/inputfmt` package — magic-byte classifier

**Files:**
- Create: `internal/inputfmt/inputfmt.go`
- Test: `internal/inputfmt/inputfmt_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/inputfmt/inputfmt_test.go`:

```go
package inputfmt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDetect(t *testing.T) {
	cases := []struct {
		name   string
		header []byte
		want   Kind
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, KindJPEG},
		{"png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}, KindPNG},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), KindWebP},
		{"tiff_le_is_raw_fallthrough", []byte{0x49, 0x49, 0x2A, 0x00, 0x10, 0x00}, KindRAW},
		{"tiff_be_is_raw_fallthrough", []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x10}, KindRAW},
		{"riff_not_webp", []byte("RIFF\x00\x00\x00\x00AVI ....."), KindRAW},
		{"too_short", []byte{0xFF, 0xD8}, KindRAW},
		{"empty", []byte{}, KindRAW},
		{"random", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}, KindRAW},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, Detect(c.header))
		})
	}
}

func TestKindDirect(t *testing.T) {
	require.True(t, KindJPEG.Direct())
	require.True(t, KindPNG.Direct())
	require.True(t, KindWebP.Direct())
	require.False(t, KindRAW.Direct())
}
```

Note on `too_short`: JPEG needs 3 bytes, but the case asserts `KindRAW` for a 2-byte input — `Detect` must guard every signature against short slices.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/inputfmt/ -v`
Expected: FAIL — build error, `undefined: Detect`, `undefined: Kind`, `undefined: KindJPEG`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/inputfmt/inputfmt.go`:

```go
// Package inputfmt classifies image input from its leading bytes so the
// request pipeline can route web formats to libvips and everything else to
// the LibRaw path.
package inputfmt

import "bytes"

// Kind is the detected input format. KindRAW is the fall-through used for
// anything not positively recognized as a direct-decode web format — including
// TIFF, whose magic bytes collide with TIFF-based RAW formats.
type Kind int

const (
	KindRAW Kind = iota
	KindJPEG
	KindPNG
	KindWebP
)

// Direct reports whether the kind is decoded directly by libvips (skipping
// LibRaw).
func (k Kind) Direct() bool { return k != KindRAW }

var pngSig = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// Detect classifies input from its leading bytes. It positively recognizes
// JPEG, PNG, and WebP; anything else returns KindRAW. A short or empty header
// is safe and returns KindRAW.
func Detect(header []byte) Kind {
	if len(header) >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
		return KindJPEG
	}
	if len(header) >= 8 && bytes.Equal(header[:8], pngSig) {
		return KindPNG
	}
	if len(header) >= 12 && bytes.Equal(header[:4], []byte("RIFF")) && bytes.Equal(header[8:12], []byte("WEBP")) {
		return KindWebP
	}
	return KindRAW
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/inputfmt/ -v`
Expected: PASS (all subtests of `TestDetect` and `TestKindDirect`).

- [ ] **Step 5: Commit**

```bash
git add internal/inputfmt/inputfmt.go internal/inputfmt/inputfmt_test.go
git commit -m "feat(inputfmt): magic-byte classifier for JPEG/PNG/WebP"
```

---

### Task 2: Rename `vipsproc.ProcessJPEG` → `ProcessEncoded`

The method already loads any encoded buffer (`vips.NewImageFromBuffer`); only the name is JPEG-specific. Rename it and update the one existing call site so the tree keeps compiling, then prove PNG and WebP inputs work.

**Files:**
- Modify: `internal/vipsproc/pipeline.go:51` (method name + doc)
- Modify: `internal/httpapi/handler.go:212` (call site)
- Test: `internal/vipsproc/pipeline_test.go`

- [ ] **Step 1: Write the failing tests**

In `internal/vipsproc/pipeline_test.go`, rename the existing `TestProcessFromJPEGBytes_Resize` to call `ProcessEncoded`, and add PNG + WebP input cases. Replace the body of `TestProcessFromJPEGBytes_Resize` and add two new functions plus a `makePNG` helper:

```go
func TestProcessEncoded_JPEGResize(t *testing.T) {
	p := NewPipeline()

	src := makeJPEG(t, 2000, 1000)
	out, ct, err := p.ProcessEncoded(src, Options{Width: 1000, Height: 500, Fit: FitScale, Format: FormatJPEG})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ct)

	img, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	require.LessOrEqual(t, img.Bounds().Dx(), 1000)
}

func TestProcessEncoded_PNGInput(t *testing.T) {
	p := NewPipeline()

	src := makePNG(t, 800, 400)
	out, ct, err := p.ProcessEncoded(src, Options{Width: 400, Height: 200, Fit: FitScale, Format: FormatPNG})
	require.NoError(t, err)
	require.Equal(t, "image/png", ct)
	require.Greater(t, len(out), 0)
}

func TestProcessEncoded_WebPInput(t *testing.T) {
	p := NewPipeline()

	// Produce WebP bytes via the pipeline, then feed them back in as input.
	webpIn, _, err := p.ProcessImage(image.NewRGBA(image.Rect(0, 0, 800, 400)),
		Options{Format: FormatWebP})
	require.NoError(t, err)

	out, ct, err := p.ProcessEncoded(webpIn, Options{Width: 400, Height: 200, Fit: FitScale, Format: FormatJPEG})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ct)
	require.Greater(t, len(out), 0)
}

func makePNG(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
```

Add `"image/png"` to the import block of `pipeline_test.go` (alongside the existing `"image/jpeg"`). Delete the old `TestProcessFromJPEGBytes_Resize` function (it is replaced by `TestProcessEncoded_JPEGResize`).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/vipsproc/ -v`
Expected: FAIL — build error, `p.ProcessEncoded undefined (type *Pipeline has no field or method ProcessEncoded)`.

- [ ] **Step 3: Rename the method and its call site**

In `internal/vipsproc/pipeline.go`, change the method (currently at line 51):

```go
// ProcessEncoded resizes and re-encodes an already-encoded image buffer
// (JPEG, PNG, WebP, or a RAW embedded preview). The input format is detected
// by libvips from the buffer.
func (p *Pipeline) ProcessEncoded(in []byte, opts Options) ([]byte, string, error) {
	img, err := vips.NewImageFromBuffer(in)
	if err != nil {
		return nil, "", err
	}
	defer img.Close()
	if err := img.AutoRotate(); err != nil {
		return nil, "", err
	}
	return p.finish(img, opts)
}
```

In `internal/httpapi/handler.go`, update the fast-path call (currently at line 212) from `h.Vips.ProcessJPEG(data, vipsOpts(req, format))` to:

```go
				out, _, vErr := h.Vips.ProcessEncoded(data, vipsOpts(req, format))
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go build ./... && go test ./internal/vipsproc/ -v`
Expected: PASS for `TestProcessEncoded_JPEGResize`, `TestProcessEncoded_PNGInput`, `TestProcessEncoded_WebPInput`, and the unchanged `TestProcessFromImage_FormatWebP`. `go build ./...` succeeds (handler.go compiles with the renamed call).

- [ ] **Step 5: Commit**

```bash
git add internal/vipsproc/pipeline.go internal/vipsproc/pipeline_test.go internal/httpapi/handler.go
git commit -m "refactor(vipsproc): rename ProcessJPEG to ProcessEncoded"
```

---

### Task 3: Route detected web formats to the direct path

Detect the input kind after materializing and, for JPEG/PNG/WebP, decode directly with libvips (`X-Path: direct`), ignoring RAW-only params. RAW input keeps the existing fast/slow path.

**Files:**
- Modify: `internal/httpapi/handler.go` (imports + `process()` at line 200)
- Test: `internal/httpapi/handler_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/httpapi/handler_test.go`. Add these imports to the existing import block: `"bytes"`, `"image"`, `"image/jpeg"`, `"image/png"`, and `"github.com/seppedelanghe/roxy/internal/vipsproc"`.

```go
func writeJPEGFixture(t *testing.T, path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func writePNGFixture(t *testing.T, path string, w, h int) {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0644))
}

func TestProcessDirectJPEG(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writeJPEGFixture(t, filepath.Join(root, "photo.jpg"), 1600, 900)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=photo.jpg&res=720p", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/jpeg", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
	require.Equal(t, "MISS", rec.Header().Get("X-Cache"))
}

func TestProcessDirectPNG(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writePNGFixture(t, filepath.Join(root, "logo.png"), 800, 600)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=logo.png&res=480p", nil)
	req.Header.Set("Accept", "image/png")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "image/png", rec.Header().Get("Content-Type"))
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
}

func TestProcessDirectIgnoresRawParams(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	writeJPEGFixture(t, filepath.Join(root, "photo.jpg"), 1600, 900)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=photo.jpg&res=720p&wb=camera&exp=1.5", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "direct", rec.Header().Get("X-Path"))
}

func TestProcessDirectCorruptIsUnsupported(t *testing.T) {
	h, root := newTestHandler(t)
	h.Vips = vipsproc.NewPipeline()
	// Valid JPEG magic, garbage body: detected as JPEG, libvips fails to decode.
	require.NoError(t, os.WriteFile(filepath.Join(root, "bad.jpg"),
		append([]byte{0xFF, 0xD8, 0xFF}, []byte("not a real jpeg")...), 0644))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/process?file=bad.jpg", nil)
	req.Header.Set("Accept", "image/jpeg")
	h.Process(rec, req)

	require.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	require.Contains(t, rec.Body.String(), "unsupported_format")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run TestProcessDirect -v`
Expected: FAIL — `TestProcessDirectJPEG`/`PNG`/`IgnoresRawParams` fail because there is no direct branch yet: the request enters the RAW fast path, `h.RAW` is nil, and `ExtractLargestPreview` panics (nil pointer) or errors. `TestProcessDirectCorruptIsUnsupported` also fails for the same reason.

- [ ] **Step 3: Add detection + direct branch to `process()`**

In `internal/httpapi/handler.go`, add `"github.com/seppedelanghe/roxy/internal/inputfmt"` to the import block.

In `process()` (currently starting at line 200), insert the detection-and-direct branch immediately after the `defer cleanup()` line and before `wantFast := ...`:

```go
	defer cleanup()

	kind, kErr := detectKind(localPath)
	if kErr != nil {
		return nil, "", &processError{ErrInternal, kErr.Error()}
	}
	if kind.Direct() {
		data, rErr := os.ReadFile(localPath)
		if rErr != nil {
			return nil, "direct", &processError{ErrInternal, rErr.Error()}
		}
		out, _, vErr := h.Vips.ProcessEncoded(data, vipsOpts(req, format))
		if vErr != nil {
			return nil, "direct", &processError{ErrUnsupportedFormat, vErr.Error()}
		}
		return out, "direct", nil
	}

	wantFast := req.WB == "auto" && req.Exp == 0
```

Add a helper at the end of `handler.go` that reads the leading bytes and classifies them:

```go
// detectKind reads the leading bytes of a materialized source file and
// classifies its format. A short read is fine; inputfmt treats it as RAW.
func detectKind(path string) (inputfmt.Kind, error) {
	f, err := os.Open(path)
	if err != nil {
		return inputfmt.KindRAW, err
	}
	defer f.Close()
	var hdr [16]byte
	n, err := f.Read(hdr[:])
	if err != nil && err != io.EOF {
		return inputfmt.KindRAW, err
	}
	return inputfmt.Detect(hdr[:n]), nil
}
```

(`io` and `os` are already imported in `handler.go`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS for all `TestProcessDirect*` tests and the pre-existing handler tests (`TestHealthz`, `TestProcessNotFound`, `TestProcessRejectsUnknownParam`, `TestProcessTooLarge`, etc.).

- [ ] **Step 5: Run the full suite**

Run: `go build ./... && go test ./...`
Expected: All packages PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/handler.go internal/httpapi/handler_test.go
git commit -m "feat(httpapi): direct libvips path for JPEG/PNG/WebP input"
```

---

### Task 4: Update README

**Files:**
- Modify: `README.md` (lines 3, 15, 56-59 region, 130-137 region, path diagram around lines 29-50)

- [ ] **Step 1: Update the intro and supported-formats lines**

Change line 3 from:

```markdown
**RAW-first image optimization proxy** on-the-fly processing, resizing, and caching of camera RAW files (`.CR2`, `.CR3`, `.NEF`, `.ARW`, `.DNG`, and more), served over HTTP.
```

to:

```markdown
**RAW-first image optimization proxy** on-the-fly processing, resizing, and caching of camera RAW files (`.CR2`, `.CR3`, `.NEF`, `.ARW`, `.DNG`, and more) plus common web formats (JPEG, PNG, WebP), served over HTTP.
```

After the "**Supported RAW formats:**" line (line 15), add:

```markdown

**Supported non-RAW inputs:** JPEG, PNG, and WebP are detected by content and decoded directly via `libvips`, skipping LibRaw entirely.
```

- [ ] **Step 2: Update the "What roxy is not for" bullet**

Change the line 58 bullet from:

```markdown
- **Non-RAW input** - JPEG, PNG, TIFF, and video files are not (yet) supported as source material.
```

to:

```markdown
- **TIFF / video input** - JPEG, PNG, and WebP are supported as source material; TIFF (its magic bytes collide with TIFF-based RAW) and video files are not.
```

- [ ] **Step 3: Add the direct path to the diagram**

In the ASCII pipeline diagram (the fenced block around lines 29-50), add a branch after the "Open RAW via storage backend" step showing that JPEG/PNG/WebP go straight to the libvips pipeline. Replace the diagram's branch section so it reads:

```
  Open source via storage backend
       │
       ├── JPEG/PNG/WebP? ──YES──→ [ DIRECT PATH ] decode via libvips
       │                                                 │
       ├── embedded JPEG large enough? ──YES──→ [ FAST PATH ] extract preview (~1–5ms)
       │                                                 │
       └── NO ───────────────────────────────→ [ SLOW PATH ] demosaic (~100–300ms)
                                                          │
                                                          ▼
                                                  libvips pipeline
                                               (resize, crop, format)
```

- [ ] **Step 4: Note param semantics and X-Path in the API section**

After the parameters table (around line 137, before the "The output format is negotiated..." paragraph), add:

```markdown
For non-RAW input (JPEG/PNG/WebP), the RAW-only parameters (`wb`, `exp`, `embed_only`, `half_size`) are accepted but ignored.
```

In the `X-Path` row of the response-headers table (line ~158), change its description to include `direct`:

```markdown
| `X-Path` | `cache`, `direct`, `fast`, or `slow` -> which pipeline served the request |
```

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: document JPEG/PNG/WebP input support"
```

---

## Self-Review

**Spec coverage:**
- `internal/inputfmt` package (Detect/Kind/Direct) → Task 1. ✓
- `ProcessJPEG` → `ProcessEncoded` rename → Task 2. ✓
- Routing in `process()` (detect after materialize, direct branch, no semaphore, ignore RAW params) → Task 3. ✓
- Error mapping: direct decode failure → 415 → Task 3 (`TestProcessDirectCorruptIsUnsupported` + `ErrUnsupportedFormat`). ✓
- `X-Path: direct` → Task 3 (asserted) + Task 4 (documented). ✓
- TIFF excluded / falls through to RAW → Task 1 (`tiff_*_is_raw_fallthrough` cases) + Task 4 docs. ✓
- Cache key unchanged → no task needed; `cachekey.Compute` is untouched. ✓
- No new config → no task needed. ✓
- Tests for inputfmt / vipsproc / httpapi + fixtures → Tasks 1-3. ✓ (Fixtures are generated in-test rather than committed under `testdata/`, which is simpler and self-contained — a deliberate, equivalent substitution for the spec's `testdata/` note.)
- README updates → Task 4. ✓

**Placeholder scan:** No TBD/TODO/"handle edge cases"/"similar to" placeholders; every code step shows complete code. ✓

**Type consistency:** `Kind`, `KindRAW/KindJPEG/KindPNG/KindWebP`, `Detect`, `Direct()`, `ProcessEncoded`, and `detectKind` are named identically across Tasks 1-3. The Task 3 call to `ProcessEncoded` matches the signature defined in Task 2. ✓
