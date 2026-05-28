package diskcache

import (
	"container/list"
	"sync"
	"time"
)

type Meta struct {
	Size        int64
	ContentType string
	MTime       time.Time
}

type entry struct {
	key  string
	meta Meta
}

type LRU struct {
	mu     sync.Mutex
	ll     *list.List
	idx    map[string]*list.Element
	total  int64
	maxLen int
}

func NewLRU(maxLen int) *LRU {
	return &LRU{ll: list.New(), idx: make(map[string]*list.Element), maxLen: maxLen}
}

func (l *LRU) Put(key string, m Meta) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.idx[key]; ok {
		l.total -= e.Value.(*entry).meta.Size
		e.Value.(*entry).meta = m
		l.total += m.Size
		l.ll.MoveToFront(e)
		return
	}
	e := l.ll.PushFront(&entry{key: key, meta: m})
	l.idx[key] = e
	l.total += m.Size
}

func (l *LRU) Get(key string) (Meta, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.idx[key]
	if !ok {
		return Meta{}, false
	}
	l.ll.MoveToFront(e)
	return e.Value.(*entry).meta, true
}

func (l *LRU) Delete(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e, ok := l.idx[key]; ok {
		l.total -= e.Value.(*entry).meta.Size
		l.ll.Remove(e)
		delete(l.idx, key)
	}
}

func (l *LRU) TotalBytes() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.total
}

func (l *LRU) OldestKeys(n int) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, n)
	for e := l.ll.Back(); e != nil && len(out) < n; e = e.Prev() {
		out = append(out, e.Value.(*entry).key)
	}
	return out
}
