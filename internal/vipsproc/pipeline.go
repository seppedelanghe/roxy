package vipsproc

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"sync"

	"github.com/davidbyttow/govips/v2/vips"
)

type Format string

const (
	FormatJPEG Format = "jpeg"
	FormatPNG  Format = "png"
	FormatWebP Format = "webp"
)

type Fit string

const (
	FitScale Fit = "scale"
	FitCrop  Fit = "crop"
	FitNone  Fit = "none"
)

type Options struct {
	Width, Height int
	Fit           Fit
	Format        Format
	Quality       int
}

type Pipeline struct{}

var initOnce sync.Once

func NewPipeline() *Pipeline {
	initOnce.Do(func() {
		vips.Startup(&vips.Config{
			ConcurrencyLevel: 1,
			MaxCacheSize:     0,
			MaxCacheMem:      0,
		})
	})
	return &Pipeline{}
}

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

func (p *Pipeline) ProcessImage(src image.Image, opts Options) ([]byte, string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil, "", err
	}
	img, err := vips.NewImageFromBuffer(buf.Bytes())
	if err != nil {
		return nil, "", err
	}
	defer img.Close()
	return p.finish(img, opts)
}

func (p *Pipeline) finish(img *vips.ImageRef, opts Options) ([]byte, string, error) {
	if opts.Width > 0 && opts.Height > 0 {
		switch opts.Fit {
		case FitScale, FitNone:
			if err := img.Thumbnail(opts.Width, opts.Height, vips.InterestingNone); err != nil {
				return nil, "", err
			}
		case FitCrop:
			if err := img.Thumbnail(opts.Width, opts.Height, vips.InterestingCentre); err != nil {
				return nil, "", err
			}
		default:
			return nil, "", errors.New("vipsproc: unknown fit")
		}
	}
	_ = img.RemoveOrientation()

	q := opts.Quality
	if q == 0 {
		q = 80
	}
	switch opts.Format {
	case FormatJPEG:
		b, _, err := img.ExportJpeg(&vips.JpegExportParams{Quality: q, StripMetadata: true})
		return b, "image/jpeg", err
	case FormatPNG:
		b, _, err := img.ExportPng(&vips.PngExportParams{Compression: 6, StripMetadata: true})
		return b, "image/png", err
	case FormatWebP:
		b, _, err := img.ExportWebp(&vips.WebpExportParams{Quality: q, StripMetadata: true})
		return b, "image/webp", err
	}
	return nil, "", errors.New("vipsproc: unknown format")
}
