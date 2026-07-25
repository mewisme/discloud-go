package cache

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryURLAndTTL(t *testing.T) {
	m := NewMemory()
	defer m.Close()
	ex := time.Now().Add(10 * time.Minute).Unix()
	cdn := "https://cdn.discordapp.com/x?ex=" + // hex expiry
		formatHex(ex)
	m.SetURL(context.Background(), "mid", cdn)
	v, ok := m.GetURL(context.Background(), "mid")
	if !ok || v != cdn {
		t.Fatalf("got %q ok=%v", v, ok)
	}
}

func formatHex(n int64) string {
	const hexdigits = "0123456789abcdef"
	if n == 0 {
		return "0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = hexdigits[n&15]
		n >>= 4
	}
	return string(buf[i:])
}

func TestMemoryCounters(t *testing.T) {
	m := NewMemory()
	defer m.Close()
	ctx := context.Background()
	n, err := m.Incr(ctx, "k")
	if err != nil || n != 1 {
		t.Fatalf("%d %v", n, err)
	}
	n, err = m.IncrBy(ctx, "k", 5)
	if err != nil || n != 6 {
		t.Fatalf("%d %v", n, err)
	}
	if err := m.Expire(ctx, "k", time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	// After expiry, next incr starts fresh
	n, _ = m.Incr(ctx, "k")
	if n != 1 {
		t.Fatalf("after expire got %d", n)
	}
}

func TestMemoryConcurrency(t *testing.T) {
	m := NewMemory()
	defer m.Close()
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.Incr(ctx, "c")
			m.SetURL(ctx, "m", "https://example.com/?ex=ffffffff")
			_, _ = m.GetURL(ctx, "m")
		}()
	}
	wg.Wait()
	if err := m.Ping(ctx); err != nil {
		t.Fatal(err)
	}
}
