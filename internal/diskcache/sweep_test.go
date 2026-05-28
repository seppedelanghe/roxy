package diskcache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSweepRemovesTmpAndRebuildsLRU(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k1"), []byte("aa"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k2"), []byte("bbb"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "k3.abcd.tmp"), []byte("zzzz"), 0644))

	lru := NewLRU(0)
	require.NoError(t, Sweep(dir, lru))

	_, err := os.Stat(filepath.Join(dir, "k3.abcd.tmp"))
	require.ErrorIs(t, err, os.ErrNotExist)

	require.Equal(t, int64(5), lru.TotalBytes())
}

func TestSweepCreatesDirIfMissing(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "new-cache")
	require.NoError(t, Sweep(dir, NewLRU(0)))
	info, err := os.Stat(dir)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}
