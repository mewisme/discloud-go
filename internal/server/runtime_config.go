package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/mewisme/discloud-go/internal/appconfig"
	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) effectiveChunkSize() int64 {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	if s.chunkSize <= 0 {
		return defaultChunkSize
	}
	return s.chunkSize
}

// RefreshPublicConfig reloads allowlisted settings from env+DB (Postgres) and
// re-resolves chunk size. No-op when UseDBConfig is false (local/SQLite).
func (s *Server) RefreshPublicConfig(ctx context.Context) error {
	if !s.useDBConfig {
		return nil
	}
	rows, err := s.store.ListConfig(ctx)
	if err != nil {
		if errors.Is(err, store.ErrNotSupported) {
			return nil
		}
		return err
	}
	envDefaults := s.envPublic
	if envDefaults == nil {
		envDefaults = map[string]string{}
	}
	values, sources, updated := appconfig.Merge(envDefaults, rows)
	return s.applyMergedPublic(ctx, values, sources, updated)
}

func (s *Server) applyMergedPublic(ctx context.Context, values map[string]string, sources map[string]appconfig.Source, updated map[string]string) error {
	guildID := strings.TrimSpace(values[appconfig.KeyGuildID])
	preferred, err := appconfig.ParseInt64(values[appconfig.KeyChunkSize])
	if err != nil {
		return err
	}
	size, _, err := appconfig.ResolveEffectiveChunk(ctx, s.discord, guildID, preferred)
	if err != nil {
		return err
	}

	rlUp, err := appconfig.ParseInt(values[appconfig.KeyRLUpload])
	if err != nil {
		return err
	}
	rlDown, err := appconfig.ParseInt(values[appconfig.KeyRLDownload])
	if err != nil {
		return err
	}
	maxUser, err := appconfig.ParseInt64(values[appconfig.KeyMaxUserBytes])
	if err != nil {
		return err
	}
	maxAnonUp, err := appconfig.ParseInt(values[appconfig.KeyMaxAnonUploads])
	if err != nil {
		return err
	}
	maxAnonBytes, err := appconfig.ParseInt64(values[appconfig.KeyMaxAnonBytes])
	if err != nil {
		return err
	}
	maxRaw, err := appconfig.ParseInt64(values[appconfig.KeyMaxRawUpload])
	if err != nil {
		return err
	}

	srcStr := make(map[string]string, len(sources))
	for k, v := range sources {
		srcStr[k] = string(v)
	}

	s.cfgMu.Lock()
	defer s.cfgMu.Unlock()
	s.guildID = guildID
	s.preferredChunkSize = preferred
	s.chunkSize = size
	s.dbSources = srcStr
	s.dbUpdated = updated
	s.rateLimitUploadPerMin = rlUp
	s.rateLimitDownloadPerMin = rlDown
	s.maxUserBytes = maxUser
	s.maxAnonUploadsPerDay = maxAnonUp
	s.maxAnonBytesPerDay = maxAnonBytes
	s.maxRawUploadConfigured = maxRaw
	if maxRaw <= 0 {
		s.maxRawUploadBytes = int64(maxChunksPerFile) * size
	} else {
		s.maxRawUploadBytes = maxRaw
	}
	return nil
}

func (s *Server) handleAdminGetConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireScope(w, r, store.ScopeAdmin); !ok {
		return
	}
	if !s.useDBConfig {
		writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
		return
	}
	rows, err := s.store.ListConfig(r.Context())
	if err != nil {
		if errors.Is(err, store.ErrNotSupported) {
			writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
			return
		}
		s.log.Error("list config failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	envDefaults := s.envPublic
	if envDefaults == nil {
		envDefaults = map[string]string{}
	}
	values, sources, updated := appconfig.Merge(envDefaults, rows)
	writeJSON(w, http.StatusOK, map[string]any{
		"entries": appconfig.Entries(values, sources, updated),
	})
}

func (s *Server) handleAdminPutConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireScope(w, r, store.ScopeAdmin); !ok {
		return
	}
	if !s.useDBConfig {
		writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
		return
	}
	var req struct {
		Entries map[string]string `json:"entries"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(req.Entries) == 0 {
		writeJSONError(w, http.StatusBadRequest, "entries required")
		return
	}
	clean := make(map[string]string, len(req.Entries))
	for k, v := range req.Entries {
		k = strings.TrimSpace(k)
		if !appconfig.IsAllowlisted(k) {
			writeJSONError(w, http.StatusBadRequest, "unknown config key: "+k)
			return
		}
		clean[k] = v
	}
	if err := s.store.SetConfigBatch(r.Context(), clean); err != nil {
		if errors.Is(err, store.ErrNotSupported) {
			writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
			return
		}
		s.log.Error("set config failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to save config")
		return
	}
	if err := s.RefreshPublicConfig(r.Context()); err != nil {
		s.log.Error("refresh config after put failed", "error", err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleAdminGetConfig(w, r)
}

func (s *Server) handleAdminDeleteConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireScope(w, r, store.ScopeAdmin); !ok {
		return
	}
	if !s.useDBConfig {
		writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
		return
	}
	key := strings.TrimSpace(r.PathValue("key"))
	if !appconfig.IsAllowlisted(key) {
		writeJSONError(w, http.StatusBadRequest, "unknown config key: "+key)
		return
	}
	if err := s.store.DeleteConfig(r.Context(), key); err != nil {
		if errors.Is(err, store.ErrNotSupported) {
			writeJSONError(w, http.StatusNotImplemented, "Runtime config requires Postgres")
			return
		}
		if errors.Is(err, store.ErrNotFound) {
			// Already using env — still refresh and return OK view.
		} else {
			s.log.Error("delete config failed", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to delete config")
			return
		}
	}
	if err := s.RefreshPublicConfig(r.Context()); err != nil {
		s.log.Error("refresh config after delete failed", "error", err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.handleAdminGetConfig(w, r)
}
