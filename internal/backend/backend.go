package backend

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound   = errors.New("backend: object not found")
	ErrInvalidKey = errors.New("backend: invalid key")
)

type Stat struct {
	Size    int64
	ModTime time.Time
}

type Object interface {
	io.ReaderAt
	io.Closer
}

type Backend interface {
	Open(ctx context.Context, key string) (Object, Stat, error)
	Stat(ctx context.Context, key string) (Stat, error)
	ID() string
}
