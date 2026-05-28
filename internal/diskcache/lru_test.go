package diskcache

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLRUPutGetUpdatesRecency(t *testing.T) {
	l := NewLRU(0)
	l.Put("a", Meta{Size: 10, ContentType: "image/jpeg", MTime: time.Unix(0, 1)})
	l.Put("b", Meta{Size: 20, ContentType: "image/jpeg", MTime: time.Unix(0, 2)})
	m, ok := l.Get("a")
	require.True(t, ok)
	require.Equal(t, int64(10), m.Size)
	require.Equal(t, int64(30), l.TotalBytes())
	keys := l.OldestKeys(1)
	require.Equal(t, []string{"b"}, keys)
}

func TestLRUDelete(t *testing.T) {
	l := NewLRU(0)
	l.Put("a", Meta{Size: 5})
	l.Delete("a")
	_, ok := l.Get("a")
	require.False(t, ok)
	require.Equal(t, int64(0), l.TotalBytes())
}
