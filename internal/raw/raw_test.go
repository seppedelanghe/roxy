package raw

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func samplePath(t *testing.T) string {
	t.Helper()
	const p = "testdata/sample.NEF"
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no %s available: %v", p, err)
	}
	return p
}

func TestExtractLargestPreviewReturnsJPEG(t *testing.T) {
	path := samplePath(t)
	a := NewAdapter()
	data, info, err := a.ExtractLargestPreview(context.Background(), path)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	require.Greater(t, info.LongestEdge, 0)
	require.True(t, info.IsJPEG)
}

func TestDemosaicReturnsImage(t *testing.T) {
	path := samplePath(t)
	a := NewAdapter()
	img, meta, err := a.Demosaic(context.Background(), path, DemosaicOptions{HalfSize: true})
	require.NoError(t, err)
	require.NotNil(t, img)
	require.Greater(t, meta.Width, 0)
}
