package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/store"
)

func writePasswordError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPasswordRequired):
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message": "Password required",
			"code":    "password_required",
		})
	case errors.Is(err, errPasswordInvalid):
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message": "Invalid password",
			"code":    "password_invalid",
		})
	default:
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
	}
}

func writePasswordErrorPlain(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPasswordRequired):
		http.Error(w, "Password required", http.StatusUnauthorized)
	case errors.Is(err, errPasswordInvalid):
		http.Error(w, "Invalid password", http.StatusUnauthorized)
	default:
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}
}

func (s *Server) handleShareSettings(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	f, err := s.store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !s.canManageFile(u, f) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}

	var body struct {
		Password     *string `json:"password"`
		ExpiresAt    *string `json:"expiresAt"`
		MaxDownloads *int    `json:"maxDownloads"`
		ShareMode    *string `json:"shareMode"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	patch := store.FileSharePatch{}
	now := s.now().UTC()

	if body.Password != nil {
		if *body.Password == "" {
			patch.ClearPassword = true
		} else {
			if len(*body.Password) < auth.MinPasswordLen {
				writeJSONError(w, http.StatusBadRequest, "Password must be at least 8 characters")
				return
			}
			hash, err := auth.HashPassword(*body.Password)
			if err != nil {
				s.log.Error("hash share password failed", "error", err)
				writeJSONError(w, http.StatusInternalServerError, "Internal server error")
				return
			}
			patch.PasswordHash = &hash
		}
	}

	if body.ExpiresAt != nil {
		if *body.ExpiresAt == "" {
			writeJSONError(w, http.StatusBadRequest, "expiresAt cannot be empty")
			return
		}
		exp, err := time.Parse(time.RFC3339, *body.ExpiresAt)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "expiresAt must be RFC3339")
			return
		}
		exp = exp.UTC()
		capAt := now.Add(maxRetentionFromNow)
		if !exp.After(now) || exp.After(capAt) {
			writeJSONError(w, http.StatusBadRequest, "expiresAt must be between now and 30 days from now")
			return
		}
		patch.ExpiresAt = &exp
	}

	if body.MaxDownloads != nil {
		if *body.MaxDownloads <= 0 {
			patch.ClearMaxDownloads = true
		} else {
			n := *body.MaxDownloads
			patch.MaxDownloads = &n
		}
	}

	if body.ShareMode != nil {
		mode := *body.ShareMode
		if mode != store.ShareModeView && mode != store.ShareModeDownload {
			writeJSONError(w, http.StatusBadRequest, "shareMode must be view or download")
			return
		}
		patch.ShareMode = &mode
	}

	if err := s.store.UpdateFileShare(r.Context(), id, patch); err != nil {
		s.log.Error("update share failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	f, err = s.store.GetFile(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, s.fileLinksResponse(s.baseURL(r), f, "", &u))
}

func (s *Server) handleUnlockFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	f, err := s.store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	now := s.now().UTC()
	if !f.ExpiresAt.After(now) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if f.PasswordHash == "" {
		writeJSONError(w, http.StatusBadRequest, "File is not password-protected")
		return
	}

	// Private files still need a valid token (or owner session) before unlock.
	if f.Visibility == store.VisibilityPrivate {
		u, ok := s.sessionUser(r)
		manager := ok && s.canManageFile(u, f)
		if !manager {
			raw := r.URL.Query().Get("token")
			if raw == "" {
				raw = r.Header.Get("X-File-Token")
			}
			if raw == "" || f.AccessTokenHash == "" || !s.keys.FileTokenMatch(raw, f.AccessTokenHash) {
				writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
				return
			}
		}
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if !auth.VerifyPassword(f.PasswordHash, body.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"message": "Invalid password",
			"code":    "password_invalid",
		})
		return
	}

	exp := now.Add(fileUnlockTTL).Unix()
	val := s.keys.SignFileUnlock(f.ID, exp)
	http.SetCookie(w, &http.Cookie{
		Name:     fileUnlockCookieName(f.ID),
		Value:    val,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(fileUnlockTTL.Seconds()),
		Expires:  now.Add(fileUnlockTTL),
	})
	writeJSON(w, http.StatusOK, map[string]any{"unlocked": true, "expiresIn": int(fileUnlockTTL.Seconds())})
}

func (s *Server) handleRevokeFile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	f, err := s.store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !s.canManageFile(u, f) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	now := s.now().UTC()
	if err := s.store.RevokeFile(r.Context(), id, now); err != nil {
		s.log.Error("revoke file failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true, "fileId": id})
}
