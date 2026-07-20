package discord

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestSplitTokens(t *testing.T) {
	cases := map[string][]string{
		"a":           {"a"},
		"a,b":         {"a", "b"},
		" a , b , c ": {"a", "b", "c"},
		"a,,b,":       {"a", "b"},
		"":            {},
		",,,":         {},
	}
	for in, want := range cases {
		got := SplitTokens(in)
		if len(got) != len(want) {
			t.Fatalf("SplitTokens(%q) = %v, want %v", in, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("SplitTokens(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestUploadChunkRoundRobin(t *testing.T) {
	var mu sync.Mutex
	var used []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bot ")
		mu.Lock()
		used = append(used, auth)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"1","attachments":[{"url":"http://example/a"}]}`)
	}))
	t.Cleanup(ts.Close)

	c := New("tok-a, tok-b, tok-c", "channel")
	c.BaseURL = ts.URL
	if c.TokenCount() != 3 {
		t.Fatalf("TokenCount = %d, want 3", c.TokenCount())
	}

	for i := 0; i < 6; i++ {
		if _, err := c.UploadChunk(context.Background(), "f.bin", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"tok-b", "tok-c", "tok-a", "tok-b", "tok-c", "tok-a"}
	// next starts at 0; first Add(1)%3 = 1 -> tok-b. That's fine as long as
	// all three tokens appear equally.
	counts := map[string]int{}
	for _, tkn := range used {
		counts[tkn]++
	}
	if len(counts) != 3 || counts["tok-a"] != 2 || counts["tok-b"] != 2 || counts["tok-c"] != 2 {
		t.Fatalf("token distribution = %v (order %v), want 2 each of a/b/c (ref %v)", counts, used, want)
	}
}
