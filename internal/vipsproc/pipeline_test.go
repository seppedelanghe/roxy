package vipsproc

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessFromJPEGBytes_Resize(t *testing.T) {
	p := NewPipeline()

	src := makeJPEG(t, 2000, 1000)
	out, ct, err := p.ProcessJPEG(src, Options{Width: 1000, Height: 500, Fit: FitScale, Format: FormatJPEG})
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", ct)

	img, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	require.LessOrEqual(t, img.Bounds().Dx(), 1000)
}

func TestProcessFromImage_FormatWebP(t *testing.T) {
	p := NewPipeline()

	src := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for x := 0; x < 800; x++ {
		src.Set(x, 200, color.RGBA{255, 0, 0, 255})
	}
	out, ct, err := p.ProcessImage(src, Options{Width: 400, Height: 200, Fit: FitScale, Format: FormatWebP})
	require.NoError(t, err)
	require.Equal(t, "image/webp", ct)
	require.Greater(t, len(out), 0)
}

func makeJPEG(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}))
	return buf.Bytes()
}
