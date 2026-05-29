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
