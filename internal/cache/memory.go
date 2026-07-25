package cache

import (
	"context"
	"sync"
	"time"
)

// Memory is a single-process, concurrency-safe cache. It is not distributed.
type Memory struct {
	mu       sync.Mutex
	urls     map[string]memEntry
	counters map[string]memCounter
	stop     chan struct{}
	wg       sync.WaitGroup
}

type memEntry struct {
	value  string
	expiry time.Time
}

type memCounter struct {
	value  int64
	expiry time.Time
	hasTTL bool
}

// NewMemory creates an in-memory cache with a background cleanup loop.
func NewMemory() *Memory {
	m := &Memory{
		urls:     make(map[string]memEntry),
		counters: make(map[string]memCounter),
		stop:     make(chan struct{}),
	}
	m.wg.Add(1)
	go m.cleanupLoop()
	return m
}

func (m *Memory) Close() {
	select {
	case <-m.stop:
		return
	default:
		close(m.stop)
	}
	m.wg.Wait()
}

func (m *Memory) Ping(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (m *Memory) GetURL(ctx context.Context, messageID string) (string, bool) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.urls[key(messageID)]
	if !ok || (!e.expiry.IsZero() && time.Now().After(e.expiry)) {
		delete(m.urls, key(messageID))
		return "", false
	}
	return e.value, true
}

func (m *Memory) SetURL(ctx context.Context, messageID, cdnURL string) {
	_ = ctx
	ttl := TTLFromSignedURL(cdnURL, time.Now())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.urls[key(messageID)] = memEntry{
		value:  cdnURL,
		expiry: time.Now().Add(ttl),
	}
}

func (m *Memory) Incr(ctx context.Context, k string) (int64, error) {
	return m.IncrBy(ctx, k, 1)
}

func (m *Memory) IncrBy(ctx context.Context, k string, n int64) (int64, error) {
	_ = ctx
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.counters[k]
	if !ok || (c.hasTTL && time.Now().After(c.expiry)) {
		c = memCounter{}
	}
	c.value += n
	m.counters[k] = c
	return c.value, nil
}

func (m *Memory) Expire(ctx context.Context, k string, ttl time.Duration) error {
	_ = ctx
	if ttl <= 0 {
		ttl = time.Second
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.counters[k]
	c.hasTTL = true
	c.expiry = time.Now().Add(ttl)
	m.counters[k] = c
	return nil
}

func (m *Memory) cleanupLoop() {
	defer m.wg.Done()
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-t.C:
			m.purgeExpired()
		}
	}
}

func (m *Memory) purgeExpired() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.urls {
		if !e.expiry.IsZero() && now.After(e.expiry) {
			delete(m.urls, k)
		}
	}
	for k, c := range m.counters {
		if c.hasTTL && now.After(c.expiry) {
			delete(m.counters, k)
		}
	}
}
