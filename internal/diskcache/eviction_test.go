package diskcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEvictUntilUnderTarget(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, NewLRU(0))
	for _, k := range []string{"k1", "k2", "k3"} {
		require.NoError(t, c.Store(k, []byte("xxxxxxxxxx"), "image/jpeg"))
	}
	EvictToTarget(c, 15, 5)
	require.LessOrEqual(t, c.lru.TotalBytes(), int64(10))
	_, err := os.Stat(filepath.Join(dir, "k1"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestEvictTTL(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, NewLRU(0))
	require.NoError(t, c.Store("old", []byte("aa"), "image/jpeg"))
	past := time.Now().Add(-10 * 24 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "old"), past, past))
	c.lru.Delete("old")
	c.lru.Put("old", Meta{Size: 2, MTime: past})

	require.NoError(t, c.Store("new", []byte("bb"), "image/jpeg"))

	EvictTTL(c, 7*24*time.Hour)
	_, err := os.Stat(filepath.Join(dir, "old"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "new"))
	require.NoError(t, err)
}
