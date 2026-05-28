package backend

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalOpenReadsBytes(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.bin"), []byte("hello"), 0644))
	b, err := NewLocal(root)
	require.NoError(t, err)

	obj, stat, err := b.Open(context.Background(), "a.bin")
	require.NoError(t, err)
	defer obj.Close()
	require.Equal(t, int64(5), stat.Size)

	buf := make([]byte, 5)
	n, err := obj.ReadAt(buf, 0)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.Equal(t, "hello", string(buf))
}

func TestLocalStatMissing(t *testing.T) {
	root := t.TempDir()
	b, _ := NewLocal(root)
	_, err := b.Stat(context.Background(), "missing.bin")
	require.True(t, errors.Is(err, ErrNotFound))
}

func TestLocalRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	b, _ := NewLocal(root)
	_, _, err := b.Open(context.Background(), "../etc/passwd")
	require.True(t, errors.Is(err, ErrInvalidKey))
	_, _, err = b.Open(context.Background(), "/etc/passwd")
	require.True(t, errors.Is(err, ErrInvalidKey))
}

func TestLocalIDStable(t *testing.T) {
	b, _ := NewLocal(t.TempDir())
	require.Equal(t, b.ID(), b.ID())
	require.Contains(t, b.ID(), "local:")
}
