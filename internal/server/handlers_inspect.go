package server

import (
	"errors"
	"net/http"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	info, err := s.store.GetFileInspect(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		s.log.Error("inspect failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	base := s.baseURL(r)
	links := fileLinks(base, info.ID, info.Name, info.Size)
	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":          info.ID,
		"fileName":        info.Name,
		"fileSize":        info.Size,
		"chunkSize":       info.ChunkSize,
		"chunkCount":      info.ChunkCount,
		"createdAt":       info.CreatedAt,
		"views":           info.Views,
		"downloads":       info.Downloads,
		"ranges":          info.Ranges,
		"bytesServed":     info.BytesServed,
		"uniqueVisitors":  info.UniqueVisitors,
		"lastAccessAt":    info.LastAccessAt,
		"url":             links["url"],
		"longURL":         links["longURL"],
		"downloadURL":     links["downloadURL"],
		"longDownloadURL": links["longDownloadURL"],
	})
}
