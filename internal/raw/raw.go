package raw

import (
	"context"
	"errors"
	"image"
	"math"

	golibraw "github.com/seppedelanghe/go-libraw"
)

var ErrNoPreview = errors.New("raw: no embedded preview")

type PreviewInfo struct {
	Width       int
	Height      int
	LongestEdge int
	IsJPEG      bool
}

type DemosaicOptions struct {
	HalfSize      bool
	WhiteBalance  string  // "auto", "camera", "daylight"
	ExposureStops float64 // applied as exp_shift = 2^stops
}

type DemosaicMeta struct {
	Width  int
	Height int
}

type Adapter struct{}

func NewAdapter() *Adapter { return &Adapter{} }

func (a *Adapter) ExtractLargestPreview(ctx context.Context, path string) ([]byte, PreviewInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, PreviewInfo{}, err
	}
	p := golibraw.NewProcessor(golibraw.NewProcessorOptions())
	data, info, err := p.ExtractLargestPreview(path)
	if errors.Is(err, golibraw.ErrNoPreview) {
		return nil, PreviewInfo{}, ErrNoPreview
	}
	if err != nil {
		return nil, PreviewInfo{}, err
	}
	longest := info.Width
	if info.Height > longest {
		longest = info.Height
	}
	return data, PreviewInfo{
		Width:       info.Width,
		Height:      info.Height,
		LongestEdge: longest,
		IsJPEG:      info.Format == golibraw.PreviewJPEG,
	}, nil
}

func (a *Adapter) Demosaic(ctx context.Context, path string, opts DemosaicOptions) (image.Image, DemosaicMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, DemosaicMeta{}, err
	}
	po := golibraw.NewProcessorOptions()
	po.HalfSize = opts.HalfSize
	po.UserFlip = -1 // let LibRaw autorotate based on EXIF
	switch opts.WhiteBalance {
	case "", "auto":
		po.UseAutoWb = true
	case "camera":
		po.UseCameraWb = true
	case "daylight":
		po.UseAutoWb = false
		po.UseCameraWb = false
	default:
		return nil, DemosaicMeta{}, errors.New("raw: invalid wb")
	}
	if opts.ExposureStops != 0 {
		po.ExpShift = float32(math.Exp2(opts.ExposureStops))
		po.ExpCorrect = true
	}
	p := golibraw.NewProcessor(po)
	img, meta, err := p.ProcessRaw(path)
	if err != nil {
		return nil, DemosaicMeta{}, err
	}
	return img, DemosaicMeta{Width: int(meta.Sizes.Width), Height: int(meta.Sizes.Height)}, nil
}
