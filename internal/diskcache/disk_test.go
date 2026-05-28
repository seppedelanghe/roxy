package diskcache

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoreThenReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, NewLRU(0))
	require.NoError(t, c.Store("abc", []byte("payload"), "image/jpeg"))
	rd, meta, ok := c.Open("abc")
	require.True(t, ok)
	defer rd.Close()
	require.Equal(t, int64(7), meta.Size)
	require.Equal(t, "image/jpeg", meta.ContentType)
	buf := make([]byte, 7)
	_, err := rd.Read(buf)
	require.NoError(t, err)
	require.Equal(t, "payload", string(buf))
}

func TestStoreUsesAtomicRename(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, NewLRU(0))
	require.NoError(t, c.Store("k", []byte("hi"), "image/png"))
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		require.NotContains(t, e.Name(), ".tmp")
	}
	require.FileExists(t, filepath.Join(dir, "k"))
}
