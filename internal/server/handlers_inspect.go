package server

import (
	"errors"
	"net/http"

	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	access, err := s.authorizeFileAccess(r, id)
	if errors.Is(err, errInvalidID) {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		s.log.Error("inspect failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if access.ViaToken {
		w.Header().Set("Referrer-Policy", "no-referrer")
	}

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

	token := ""
	if access.ViaToken {
		token = r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-File-Token")
		}
	}
	base := s.baseURL(r)
	links := s.fileLinksResponse(base, info.File, token, access.User)
	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":             info.ID,
		"fileName":           info.Name,
		"fileSize":           info.Size,
		"chunkSize":          info.ChunkSize,
		"chunkCount":         info.ChunkCount,
		"createdAt":          info.CreatedAt,
		"expiresAt":          info.ExpiresAt,
		"visibility":         info.Visibility,
		"status":             fileStatusOrReady(info.Status),
		"ownedByCurrentUser": links["ownedByCurrentUser"],
		"views":              info.Views,
		"downloads":          info.Downloads,
		"ranges":             info.Ranges,
		"bytesServed":        info.BytesServed,
		"uniqueVisitors":     info.UniqueVisitors,
		"lastAccessAt":       info.LastAccessAt,
		"url":                links["url"],
		"longURL":            links["longURL"],
		"downloadURL":        links["downloadURL"],
		"longDownloadURL":    links["longDownloadURL"],
	})
}
