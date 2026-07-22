package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/store"
)

type authBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	if !s.allowAuth(r, "signup") {
		writeJSONError(w, http.StatusTooManyRequests, "Too many requests")
		return
	}
	var body authBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	username := auth.NormalizeUsername(body.Username)
	if !auth.ValidUsername(username) {
		writeJSONError(w, http.StatusBadRequest, "Invalid username")
		return
	}
	if len(body.Password) < auth.MinPasswordLen {
		writeJSONError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		s.log.Error("hash password failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	id := newID()
	u, err := s.store.CreateUser(r.Context(), id, username, hash)
	if errors.Is(err, store.ErrConflict) {
		writeJSONError(w, http.StatusConflict, "Username already registered")
		return
	}
	if err != nil {
		s.log.Error("create user failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := s.issueSession(w, r, u); err != nil {
		s.log.Error("create session failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("signup", "user", u.ID, "role", u.Role)
	writeJSON(w, http.StatusOK, userDTO(u))
}

func (s *Server) handleSignin(w http.ResponseWriter, r *http.Request) {
	if !s.allowAuth(r, "signin") {
		writeJSONError(w, http.StatusTooManyRequests, "Too many requests")
		return
	}
	var body authBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	username := auth.NormalizeUsername(body.Username)
	u, err := s.store.GetUserByUsername(r.Context(), username)
	if err != nil || !auth.VerifyPassword(u.PasswordHash, body.Password) {
		s.log.Info("signin failed", "username", username)
		writeJSONError(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if err := s.issueSession(w, r, u); err != nil {
		s.log.Error("create session failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("signin", "user", u.ID)
	writeJSON(w, http.StatusOK, userDTO(u))
}

func (s *Server) handleSignout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		_ = s.store.DeleteSession(r.Context(), s.keys.HashSessionToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	now := s.now().UTC()
	tokenHash := s.keys.HashSessionToken(c.Value)
	u, err := s.store.GetUserBySessionHash(r.Context(), tokenHash, now)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	ip := clientIP(r)
	ua := r.Header.Get("User-Agent")
	_ = s.store.TouchSession(r.Context(), tokenHash, ip, ua, now)
	sess, err := s.store.GetSessionByTokenHash(r.Context(), tokenHash, now)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	stats, err := s.store.OwnerFileStats(r.Context(), u.ID, now, 7*24*time.Hour)
	if err != nil {
		s.log.Error("owner stats failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	defVis := u.DefaultVisibility
	if defVis == "" {
		defVis = store.VisibilityPublic
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                u.ID,
		"username":          u.Username,
		"role":              u.Role,
		"createdAt":         u.CreatedAt,
		"passwordChangedAt": u.PasswordChangedAt,
		"stats": map[string]any{
			"fileCount":         stats.FileCount,
			"totalBytes":        stats.TotalBytes,
			"publicCount":       stats.PublicCount,
			"privateCount":      stats.PrivateCount,
			"expiringSoonCount": stats.ExpiringSoonCount,
		},
		"session": map[string]any{
			"createdAt":  sess.CreatedAt,
			"lastSeenAt": sess.LastSeenAt,
			"expiresAt":  sess.ExpiresAt,
			"ip":         sess.IP,
			"userAgent":  sess.UserAgent,
		},
		"retention": map[string]any{
			"authenticatedDays":     int(authenticatedRetention / (24 * time.Hour)),
			"anonymousDays":         int(anonymousRetention / (24 * time.Hour)),
			"downloadExtensionDays": int(downloadExtension / (24 * time.Hour)),
			"maxRetentionDays":      int(maxRetentionFromNow / (24 * time.Hour)),
		},
		"preferences": map[string]any{
			"defaultVisibility": defVis,
		},
	})
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		DefaultVisibility string `json:"defaultVisibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	vis := body.DefaultVisibility
	if vis != store.VisibilityPublic && vis != store.VisibilityPrivate {
		writeJSONError(w, http.StatusBadRequest, "defaultVisibility must be public or private")
		return
	}
	if err := s.store.UpdateDefaultVisibility(r.Context(), u.ID, vis); err != nil {
		s.log.Error("update preferences failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"preferences": map[string]any{
			"defaultVisibility": vis,
		},
	})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if !auth.VerifyPassword(u.PasswordHash, body.CurrentPassword) {
		writeJSONError(w, http.StatusUnauthorized, "Current password is incorrect")
		return
	}
	if len(body.NewPassword) < auth.MinPasswordLen {
		writeJSONError(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}
	hash, err := auth.HashPassword(body.NewPassword)
	if err != nil {
		s.log.Error("hash password failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if err := s.store.UpdatePasswordHash(r.Context(), u.ID, hash); err != nil {
		s.log.Error("update password failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("password_changed", "user", u.ID)
	w.WriteHeader(http.StatusNoContent)
}

func userDTO(u store.User) map[string]any {
	return map[string]any{
		"id":       u.ID,
		"username": u.Username,
		"role":     u.Role,
	}
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, u store.User) error {
	raw, err := auth.GenerateOpaqueToken()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	ua := r.Header.Get("User-Agent")
	ip := clientIP(r)
	sess := store.Session{
		ID:         newID(),
		UserID:     u.ID,
		TokenHash:  s.keys.HashSessionToken(raw),
		ExpiresAt:  now.Add(sessionTTL),
		CreatedAt:  now,
		UserAgent:  ua,
		IP:         ip,
		LastSeenAt: now,
	}
	if err := s.store.CreateSession(r.Context(), sess); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
	})
	return nil
}

func (s *Server) sessionUser(r *http.Request) (store.User, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return store.User{}, false
	}
	u, err := s.store.GetUserBySessionHash(r.Context(), s.keys.HashSessionToken(c.Value), s.now().UTC())
	if err != nil {
		return store.User{}, false
	}
	return u, true
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	u, ok := s.sessionUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return store.User{}, false
	}
	return u, true
}

func (s *Server) allowAuth(r *http.Request, kind string) bool {
	key := "discloud:rl:" + kind + ":" + clientIP(r)
	n, err := s.cache.Incr(r.Context(), key)
	if err != nil {
		s.log.Error("rate limit incr failed", "error", err)
		return true // fail open
	}
	if n == 1 {
		_ = s.cache.Expire(r.Context(), key, authRateLimitWindow)
	}
	return n <= authRateLimit
}

// fileAccess is the result of authorizeFileAccess.
type fileAccess struct {
	File     store.File
	User     *store.User
	ViaToken bool
}

// authorizeFileAccess is the single gate for all file-read paths.
// Denials are uniform 404 (ErrNotFound).
func (s *Server) authorizeFileAccess(r *http.Request, id string) (fileAccess, error) {
	f, err := s.store.GetFile(r.Context(), id)
	if err != nil {
		return fileAccess{}, err
	}
	now := s.now().UTC()
	if !f.ExpiresAt.After(now) {
		return fileAccess{}, store.ErrNotFound
	}

	var userPtr *store.User
	if u, ok := s.sessionUser(r); ok {
		userPtr = &u
	}

	if f.Visibility == store.VisibilityPublic || f.Visibility == "" {
		return fileAccess{File: f, User: userPtr}, nil
	}

	if userPtr != nil {
		if userPtr.Role == store.RoleAdmin {
			return fileAccess{File: f, User: userPtr}, nil
		}
		if f.OwnerUserID != nil && *f.OwnerUserID == userPtr.ID {
			return fileAccess{File: f, User: userPtr}, nil
		}
	}

	raw := r.URL.Query().Get("token")
	if raw == "" {
		raw = r.Header.Get("X-File-Token")
	}
	if raw != "" && f.AccessTokenHash != "" && s.keys.FileTokenMatch(raw, f.AccessTokenHash) {
		return fileAccess{File: f, User: userPtr, ViaToken: true}, nil
	}
	return fileAccess{}, store.ErrNotFound
}

func (s *Server) canManageFile(u store.User, f store.File) bool {
	if u.Role == store.RoleAdmin {
		return true
	}
	return f.OwnerUserID != nil && *f.OwnerUserID == u.ID
}

func (s *Server) handleVisibility(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
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
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	vis := body.Visibility
	if vis != store.VisibilityPublic && vis != store.VisibilityPrivate {
		writeJSONError(w, http.StatusBadRequest, "visibility must be public or private")
		return
	}
	if f.OwnerUserID == nil && vis == store.VisibilityPrivate {
		writeJSONError(w, http.StatusForbidden, "Anonymous files cannot be private")
		return
	}

	out := map[string]any{"visibility": vis}
	if vis == store.VisibilityPublic {
		if err := s.store.UpdateVisibility(r.Context(), id, vis, nil, nil); err != nil {
			s.log.Error("update visibility failed", "id", id, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	} else {
		raw, err := auth.GenerateFileToken()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		hash := s.keys.HashFileToken(raw)
		now := s.now().UTC()
		if err := s.store.UpdateVisibility(r.Context(), id, vis, &hash, &now); err != nil {
			s.log.Error("update visibility failed", "id", id, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		out["accessToken"] = raw
	}
	s.log.Info("visibility", "file", id, "visibility", vis, "user", u.ID)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	f, err := s.store.GetFile(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !s.canManageFile(u, f) || f.Visibility != store.VisibilityPrivate {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	raw, err := auth.GenerateFileToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	now := s.now().UTC()
	if err := s.store.RotateAccessToken(r.Context(), id, s.keys.HashFileToken(raw), now); err != nil {
		s.log.Error("rotate token failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("rotate token", "file", id, "user", u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"accessToken": raw})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
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
	if err := s.store.DeleteFile(r.Context(), id); err != nil {
		s.log.Error("delete file failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("delete file", "file", id, "user", u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// RunCleanup deletes expired files periodically until ctx is cancelled.
func (s *Server) RunCleanup(ctx context.Context) {
	run := func() {
		n, err := s.store.DeleteExpiredFiles(ctx, s.now().UTC(), cleanupBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("cleanup failed", "error", err)
			return
		}
		if n > 0 {
			s.log.Info("cleanup", "deleted", n)
		}
	}
	// Shortly after start.
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
		run()
	}
	t := time.NewTicker(cleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
