package discord

import (
	"context"
	"encoding/json"
	"fmt"
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

func TestFormatParseRef(t *testing.T) {
	cases := []struct {
		msg string
		idx int
		ref string
	}{
		{"abc", 0, "abc"},
		{"abc", 1, "abc:1"},
		{"abc", 9, "abc:9"},
	}
	for _, tc := range cases {
		got := FormatRef(tc.msg, tc.idx)
		if got != tc.ref {
			t.Fatalf("FormatRef(%q,%d)=%q want %q", tc.msg, tc.idx, got, tc.ref)
		}
		m, i := ParseRef(got)
		if m != tc.msg || i != tc.idx {
			t.Fatalf("ParseRef(%q)=(%q,%d) want (%q,%d)", got, m, i, tc.msg, tc.idx)
		}
	}
	m, i := ParseRef("legacy-only")
	if m != "legacy-only" || i != 0 {
		t.Fatalf("ParseRef legacy = (%q,%d)", m, i)
	}
}

func TestUploadPartsMulti(t *testing.T) {
	var mu sync.Mutex
	var fileCount int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
			http.Error(w, "bad", 400)
			return
		}
		n := 0
		for i := 0; ; i++ {
			if _, _, err := r.FormFile(fmt.Sprintf("files[%d]", i)); err != nil {
				break
			}
			n++
		}
		mu.Lock()
		fileCount = n
		mu.Unlock()
		atts := make([]map[string]string, n)
		for i := 0; i < n; i++ {
			atts[i] = map[string]string{"url": fmt.Sprintf("http://cdn/%d", i)}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "msg1", "attachments": atts})
	}))
	t.Cleanup(ts.Close)

	c := New("tok", "channel")
	c.BaseURL = ts.URL
	refs, err := c.UploadParts(context.Background(), []Part{
		{Name: "a.bin", Data: []byte("aaa")},
		{Name: "b.bin", Data: []byte("bbb")},
		{Name: "c.bin", Data: []byte("ccc")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fileCount != 3 {
		t.Fatalf("files uploaded = %d, want 3", fileCount)
	}
	want := []string{"msg1", "msg1:1", "msg1:2"}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs[%d]=%q want %q", i, refs[i], want[i])
		}
	}
}

func TestAttachmentURLByIndex(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"m","attachments":[{"url":"http://cdn/0"},{"url":"http://cdn/1"},{"url":"http://cdn/2"}]}`)
	}))
	t.Cleanup(ts.Close)
	c := New("tok", "channel")
	c.BaseURL = ts.URL

	u0, err := c.AttachmentURL(context.Background(), "m")
	if err != nil || u0 != "http://cdn/0" {
		t.Fatalf("idx0: %q %v", u0, err)
	}
	u2, err := c.AttachmentURL(context.Background(), "m:2")
	if err != nil || u2 != "http://cdn/2" {
		t.Fatalf("idx2: %q %v", u2, err)
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
	counts := map[string]int{}
	for _, tkn := range used {
		counts[tkn]++
	}
	if len(counts) != 3 || counts["tok-a"] != 2 || counts["tok-b"] != 2 || counts["tok-c"] != 2 {
		t.Fatalf("token distribution = %v (order %v), want 2 each of a/b/c (ref %v)", counts, used, want)
	}
}
