package preset

type Fit string

const (
	FitScale Fit = "scale"
	FitCrop  Fit = "crop"
	FitNone  Fit = "none"
)

type Preset struct {
	Width  int
	Height int
	Fit    Fit
}

func (p Preset) LongestEdge() int {
	if p.Width > p.Height {
		return p.Width
	}
	return p.Height
}

var table = map[string]Preset{
	"thumb":  {150, 150, FitCrop},
	"480p":   {854, 480, FitScale},
	"720p":   {1280, 720, FitScale},
	"1080p":  {1920, 1080, FitScale},
	"source": {0, 0, FitNone},
}

func Lookup(name string) (Preset, bool) {
	p, ok := table[name]
	return p, ok
}
