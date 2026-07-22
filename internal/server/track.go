package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) trackAsync(fileID, kind string, bytes int64, r *http.Request) {
	ev := store.Event{
		FileID:      fileID,
		Kind:        kind,
		Bytes:       bytes,
		VisitorHash: s.visitorHash(r),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.store.RecordEvent(ctx, ev); err != nil {
			s.log.Error("record event failed", "file", fileID, "kind", kind, "error", err)
		}
	}()
}

func (s *Server) visitorHash(r *http.Request) string {
	ip := s.clientIP(r)
	ua := r.Header.Get("User-Agent")
	sum := sha256.Sum256([]byte(s.visitorSalt + "\x00" + ip + "\x00" + ua))
	return hex.EncodeToString(sum[:])
}

// clientIP returns the peer address used for rate limits, sessions, and visitor
// hashing. Forwarded headers are honored only when TrustProxy is set.
func (s *Server) clientIP(r *http.Request) string {
	if s.trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return strings.TrimSpace(xri)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func accessKind(r *http.Request) string {
	if r.URL.Query().Get("download") == "1" {
		return store.EventDownload
	}
	if r.Header.Get("Range") != "" {
		return store.EventRange
	}
	return store.EventView
}
