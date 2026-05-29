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
