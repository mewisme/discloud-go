package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/store"
)

var validAPIScopes = map[string]bool{
	store.ScopeUpload: true,
	store.ScopeRead:   true,
	store.ScopeManage: true,
	store.ScopeAdmin:  true,
}

func (s *Server) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireScope(w, r, store.ScopeManage)
	if !ok {
		return
	}
	var body struct {
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt *string  `json:"expiresAt"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || utf8.RuneCountInString(name) > maxAPITokenNameLen {
		writeJSONError(w, http.StatusBadRequest, "name must be 1–64 characters")
		return
	}
	scopes, err := normalizeAPIScopes(body.Scopes)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	now := s.now().UTC()
	var expiresAt *time.Time
	if body.ExpiresAt != nil && strings.TrimSpace(*body.ExpiresAt) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*body.ExpiresAt))
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "expiresAt must be RFC3339")
			return
		}
		t = t.UTC()
		if !t.After(now) {
			writeJSONError(w, http.StatusBadRequest, "expiresAt must be in the future")
			return
		}
		if t.After(now.Add(maxAPITokenTTL)) {
			writeJSONError(w, http.StatusBadRequest, "expiresAt cannot be more than 1 year from now")
			return
		}
		expiresAt = &t
	}

	n, err := s.store.CountAPITokensByUser(r.Context(), u.ID)
	if err != nil {
		s.log.Error("count api tokens failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if n >= maxAPITokensPerUser {
		writeJSONError(w, http.StatusConflict, "Maximum of 20 API tokens per user")
		return
	}

	raw, err := auth.GenerateAPIToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	tok := store.APIToken{
		ID:        newID(),
		UserID:    u.ID,
		Name:      name,
		TokenHash: s.keys.HashAPIToken(raw),
		Scopes:    scopes,
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	if err := s.store.CreateAPIToken(r.Context(), tok); err != nil {
		s.log.Error("create api token failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	out := map[string]any{
		"id":        tok.ID,
		"name":      tok.Name,
		"scopes":    tok.Scopes,
		"expiresAt": tok.ExpiresAt,
		"token":     raw,
		"createdAt": tok.CreatedAt,
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireScope(w, r, store.ScopeManage)
	if !ok {
		return
	}
	tokens, err := s.store.ListAPITokensByUser(r.Context(), u.ID)
	if err != nil {
		s.log.Error("list api tokens failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	out := make([]map[string]any, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, map[string]any{
			"id":         t.ID,
			"name":       t.Name,
			"scopes":     t.Scopes,
			"expiresAt":  t.ExpiresAt,
			"lastUsedAt": t.LastUsedAt,
			"createdAt":  t.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireScope(w, r, store.ScopeManage)
	if !ok {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid token id")
		return
	}
	now := s.now().UTC()
	if err := s.store.RevokeAPIToken(r.Context(), id, u.ID, now); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "Token not found")
			return
		}
		s.log.Error("revoke api token failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func normalizeAPIScopes(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, errors.New("scopes must be a non-empty array")
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if !validAPIScopes[s] {
			return nil, errors.New("invalid scope: " + s)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out, nil
}
