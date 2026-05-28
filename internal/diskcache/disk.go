package diskcache

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Cache struct {
	dir string
	lru *LRU
}

func New(dir string, lru *LRU) *Cache { return &Cache{dir: dir, lru: lru} }

func (c *Cache) Path(key string) string { return filepath.Join(c.dir, key) }

func (c *Cache) Open(key string) (*os.File, Meta, bool) {
	if m, ok := c.lru.Get(key); ok {
		f, err := os.Open(c.Path(key))
		if err == nil {
			return f, m, true
		}
		c.lru.Delete(key)
	}
	f, err := os.Open(c.Path(key))
	if err != nil {
		return nil, Meta{}, false
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Meta{}, false
	}
	m := Meta{Size: st.Size(), MTime: st.ModTime()}
	c.lru.Put(key, m)
	return f, m, true
}

func (c *Cache) Store(key string, body []byte, contentType string) error {
	tmpName := key + "." + randSuffix() + ".tmp"
	tmp := filepath.Join(c.dir, tmpName)
	if err := os.WriteFile(tmp, body, 0644); err != nil {
		return err
	}
	final := c.Path(key)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	c.lru.Put(key, Meta{Size: int64(len(body)), ContentType: contentType, MTime: time.Now()})
	return nil
}

func (c *Cache) Delete(key string) error {
	c.lru.Delete(key)
	err := os.Remove(c.Path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func randSuffix() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
