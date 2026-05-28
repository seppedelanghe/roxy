package preset

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLookupKnown(t *testing.T) {
	cases := map[string]Preset{
		"thumb":  {Width: 150, Height: 150, Fit: FitCrop},
		"480p":   {Width: 854, Height: 480, Fit: FitScale},
		"720p":   {Width: 1280, Height: 720, Fit: FitScale},
		"1080p":  {Width: 1920, Height: 1080, Fit: FitScale},
		"source": {Width: 0, Height: 0, Fit: FitNone},
	}
	for name, want := range cases {
		got, ok := Lookup(name)
		require.True(t, ok, name)
		require.Equal(t, want, got, name)
	}
}

func TestLookupUnknown(t *testing.T) {
	_, ok := Lookup("4k")
	require.False(t, ok)
	_, ok = Lookup("1920x1080")
	require.False(t, ok)
}

func TestLongestEdge(t *testing.T) {
	p, _ := Lookup("1080p")
	require.Equal(t, 1920, p.LongestEdge())
	p, _ = Lookup("source")
	require.Equal(t, 0, p.LongestEdge())
}
