package backend

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type Local struct {
	root string
	id   string
}

func NewLocal(root string) (*Local, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("local backend root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("local backend root %q is not a directory", abs)
	}
	return &Local{root: abs, id: "local:" + abs}, nil
}

func (l *Local) ID() string { return l.id }

func (l *Local) resolve(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") {
		return "", ErrInvalidKey
	}
	clean := filepath.Clean(key)
	if clean == "." || strings.HasPrefix(clean, "..") || strings.Contains(clean, "/../") {
		return "", ErrInvalidKey
	}
	full := filepath.Join(l.root, clean)
	rel, err := filepath.Rel(l.root, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", ErrInvalidKey
	}
	return full, nil
}

func (l *Local) Stat(ctx context.Context, key string) (Stat, error) {
	path, err := l.resolve(key)
	if err != nil {
		return Stat{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Stat{}, ErrNotFound
		}
		return Stat{}, err
	}
	return Stat{Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (l *Local) Open(ctx context.Context, key string) (Object, Stat, error) {
	path, err := l.resolve(key)
	if err != nil {
		return nil, Stat{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, Stat{}, ErrNotFound
		}
		return nil, Stat{}, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Stat{}, err
	}
	return f, Stat{Size: info.Size(), ModTime: info.ModTime()}, nil
}
