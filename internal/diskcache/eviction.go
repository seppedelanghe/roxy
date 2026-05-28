package diskcache

import (
	"context"
	"time"
)

// EvictToTarget removes oldest entries until total bytes is at most (target - headroom).
func EvictToTarget(c *Cache, target, headroom int64) {
	low := target - headroom
	if low < 0 {
		low = 0
	}
	for c.lru.TotalBytes() > low {
		keys := c.lru.OldestKeys(1)
		if len(keys) == 0 {
			return
		}
		_ = c.Delete(keys[0])
	}
}

// EvictTTL removes all entries whose mtime is older than now-ttl.
func EvictTTL(c *Cache, ttl time.Duration) {
	cutoff := time.Now().Add(-ttl)
	c.lru.mu.Lock()
	keys := make([]string, 0, len(c.lru.idx))
	mts := make([]time.Time, 0, len(c.lru.idx))
	for e := c.lru.ll.Back(); e != nil; e = e.Prev() {
		en := e.Value.(*entry)
		keys = append(keys, en.key)
		mts = append(mts, en.meta.MTime)
	}
	c.lru.mu.Unlock()
	for i, k := range keys {
		if mts[i].Before(cutoff) {
			_ = c.Delete(k)
		}
	}
}

// Run launches the background ticker. Stops when ctx is canceled.
func Run(ctx context.Context, c *Cache, maxBytes int64, ttl time.Duration, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	headroom := maxBytes / 20
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			EvictTTL(c, ttl)
			if c.lru.TotalBytes() > maxBytes {
				EvictToTarget(c, maxBytes, headroom)
			}
		}
	}
}
