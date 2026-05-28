package diskcache

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func Sweep(dir string, lru *LRU) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".tmp") {
			_ = os.Remove(filepath.Join(dir, name))
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		lru.Put(name, Meta{Size: info.Size(), MTime: info.ModTime()})
	}
	return nil
}
