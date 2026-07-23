package server

import (
	"context"
	"net/http"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) handleAdminOverview(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireScope(w, r, store.ScopeAdmin); !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	since := s.now().UTC().Add(-24 * time.Hour)
	ov, err := s.store.AdminOverview(ctx, since)
	if err != nil {
		s.log.Error("admin overview failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	pgOK := s.store.Ping(ctx) == nil
	vkOK := s.cache.Ping(ctx) == nil
	bots := 0
	if s.discord != nil {
		bots = s.discord.TokenCount()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"storage": map[string]any{
			"fileCount":       ov.FileCount,
			"totalBytes":      ov.TotalBytes,
			"chunkStoreCount": ov.ChunkStoreCount,
		},
		"users": map[string]any{
			"count":  ov.UserCount,
			"admins": ov.AdminCount,
		},
		"uploads": map[string]any{
			"openSessions": ov.OpenSessions,
			"completed24h": ov.Completed24h,
			"expired24h":   ov.Expired24h,
			"cancelled24h": ov.Cancelled24h,
		},
		"traffic": map[string]any{
			"downloads":   ov.Downloads,
			"bytesServed": ov.BytesServed,
		},
		"bots": map[string]any{
			"configured": bots,
		},
		"deps": map[string]any{
			"postgres": pgOK,
			"valkey":   vkOK,
		},
	})
}
