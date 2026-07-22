package discord

import (
	"context"
	"errors"
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
	counts := map[string]int{}
	for _, tkn := range used {
		counts[tkn]++
	}
	if len(counts) != 3 || counts["tok-a"] != 2 || counts["tok-b"] != 2 || counts["tok-c"] != 2 {
		t.Fatalf("token distribution = %v (order %v), want 2 each of a/b/c", counts, used)
	}
}

func TestAttachmentURLTriesAllBots(t *testing.T) {
	var mu sync.Mutex
	var gets []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bot ")
		mu.Lock()
		gets = append(gets, auth)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if auth == "tok-uploader" {
			io.WriteString(w, `{"id":"9","attachments":[{"url":"http://cdn/x"}]}`)
			return
		}
		io.WriteString(w, `{"id":"9","attachments":[]}`)
	}))
	t.Cleanup(ts.Close)

	c := New("tok-a, tok-uploader, tok-c", "channel")
	c.BaseURL = ts.URL
	u, err := c.AttachmentURL(context.Background(), "9", -1) // legacy: unknown bot
	if err != nil {
		t.Fatal(err)
	}
	if u != "http://cdn/x" {
		t.Fatalf("url = %q", u)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gets) < 2 || gets[len(gets)-1] != "tok-uploader" {
		t.Fatalf("gets = %v, want to end on tok-uploader", gets)
	}
}

func TestAttachmentURLUsesBotID(t *testing.T) {
	var gets []string
	var mu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := strings.TrimPrefix(r.Header.Get("Authorization"), "Bot ")
		mu.Lock()
		gets = append(gets, auth)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if auth == "tok-b" {
			io.WriteString(w, `{"id":"9","attachments":[{"url":"http://cdn/b"}]}`)
			return
		}
		io.WriteString(w, `{"id":"9","attachments":[]}`)
	}))
	t.Cleanup(ts.Close)

	c := New("tok-a, tok-b, tok-c", "ch")
	c.BaseURL = ts.URL
	u, err := c.AttachmentURL(context.Background(), "9", 1)
	if err != nil {
		t.Fatal(err)
	}
	if u != "http://cdn/b" {
		t.Fatalf("url = %q", u)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(gets) != 1 || gets[0] != "tok-b" {
		t.Fatalf("gets = %v, want only tok-b", gets)
	}
}

func TestUploadChunkReturnsBotID(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"42","attachments":[{"url":"http://cdn/x"}]}`)
	}))
	t.Cleanup(ts.Close)
	c := New("tok-a, tok-b", "ch")
	c.BaseURL = ts.URL
	up, err := c.UploadChunk(context.Background(), "f.bin", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	// first Add(1)%2 = 1 -> tok-b
	if up.MessageID != "42" || up.BotID != 1 {
		t.Fatalf("upload = %+v, want message 42 bot 1", up)
	}
}

func TestAttachmentURLGone(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)
	c := New("tok", "ch")
	c.BaseURL = ts.URL
	_, err := c.AttachmentURL(context.Background(), "missing", 0)
	if !errors.Is(err, ErrAttachmentGone) {
		t.Fatalf("err = %v, want ErrAttachmentGone", err)
	}

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"1","attachments":[]}`)
	}))
	t.Cleanup(ts2.Close)
	c.BaseURL = ts2.URL
	_, err = c.AttachmentURL(context.Background(), "empty", 0)
	if !errors.Is(err, ErrAttachmentGone) {
		t.Fatalf("empty attachments err = %v, want ErrAttachmentGone", err)
	}
}
