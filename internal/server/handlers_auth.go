package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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
	p, ok := s.currentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return
	}
	u := p.User
	now := s.now().UTC()
	ip := s.clientIP(r)
	ua := r.Header.Get("User-Agent")

	var sessionDTO map[string]any
	if !p.ViaBearer {
		c, err := r.Cookie(sessionCookieName)
		if err == nil && c.Value != "" {
			tokenHash := s.keys.HashSessionToken(c.Value)
			_ = s.store.TouchSession(r.Context(), tokenHash, ip, ua, now)
			if sess, err := s.store.GetSessionByTokenHash(r.Context(), tokenHash, now); err == nil {
				sessionDTO = map[string]any{
					"createdAt":  sess.CreatedAt,
					"lastSeenAt": sess.LastSeenAt,
					"expiresAt":  sess.ExpiresAt,
					"ip":         sess.IP,
					"userAgent":  sess.UserAgent,
				}
			}
		}
	} else {
		sessionDTO = map[string]any{
			"auth":   "bearer",
			"scopes": p.Scopes,
		}
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
	out := map[string]any{
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
		"retention": map[string]any{
			"authenticatedDays":     int(authenticatedRetention / (24 * time.Hour)),
			"anonymousDays":         int(anonymousRetention / (24 * time.Hour)),
			"downloadExtensionDays": int(downloadExtension / (24 * time.Hour)),
			"maxRetentionDays":      int(maxRetentionFromNow / (24 * time.Hour)),
		},
		"preferences": map[string]any{
			"defaultVisibility": defVis,
		},
	}
	if sessionDTO != nil {
		out["session"] = sessionDTO
	}
	if p.ViaBearer {
		out["token"] = map[string]any{
			"id":         p.TokenID,
			"name":       p.TokenName,
			"scopes":     p.Scopes,
			"expiresAt":  p.TokenExp,
			"lastUsedAt": p.TokenUsed,
			"createdAt":  p.TokenAt,
			"valid":      true,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleUpdatePreferences(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireScope(w, r, store.ScopeManage)
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
	u, ok := s.requireScope(w, r, store.ScopeManage)
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
	if err := s.store.DeleteSessionsByUserID(r.Context(), u.ID); err != nil {
		s.log.Error("revoke sessions failed", "user", u.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	_ = s.store.RevokeAPITokensByUser(r.Context(), u.ID, s.now().UTC())
	u.PasswordHash = hash
	if err := s.issueSession(w, r, u); err != nil {
		s.log.Error("reissue session failed", "user", u.ID, "error", err)
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
	ip := s.clientIP(r)
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

// authPrincipal is the resolved caller (cookie session or PAT).
type authPrincipal struct {
	User      store.User
	ViaBearer bool
	Scopes    []string // empty when cookie → all scopes
	TokenID   string
	TokenName string
	TokenExp  *time.Time
	TokenUsed *time.Time
	TokenAt   time.Time
}

func (p authPrincipal) hasScope(scope string) bool {
	if !p.ViaBearer {
		return true
	}
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func bearerRaw(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) < len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(h[len(prefix):])
}

// currentUser resolves cookie session first, then Authorization: Bearer dc_….
func (s *Server) currentUser(r *http.Request) (authPrincipal, bool) {
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		u, err := s.store.GetUserBySessionHash(r.Context(), s.keys.HashSessionToken(c.Value), s.now().UTC())
		if err == nil {
			return authPrincipal{User: u}, true
		}
	}
	raw := bearerRaw(r)
	if raw == "" || !strings.HasPrefix(raw, "dc_") {
		return authPrincipal{}, false
	}
	tok, err := s.store.GetAPITokenByHash(r.Context(), s.keys.HashAPIToken(raw))
	if err != nil || tok.RevokedAt != nil {
		return authPrincipal{}, false
	}
	now := s.now().UTC()
	if tok.ExpiresAt != nil && !tok.ExpiresAt.After(now) {
		return authPrincipal{}, false
	}
	u, err := s.store.GetUserByID(r.Context(), tok.UserID)
	if err != nil {
		return authPrincipal{}, false
	}
	if tok.LastUsedAt == nil || now.Sub(*tok.LastUsedAt) >= apiTokenTouchEvery {
		id := tok.ID
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.store.TouchAPITokenLastUsed(ctx, id, now)
		}()
	}
	return authPrincipal{
		User:      u,
		ViaBearer: true,
		Scopes:    tok.Scopes,
		TokenID:   tok.ID,
		TokenName: tok.Name,
		TokenExp:  tok.ExpiresAt,
		TokenUsed: tok.LastUsedAt,
		TokenAt:   tok.CreatedAt,
	}, true
}

// sessionUser is the identity used for ownership (uploads, file access).
func (s *Server) sessionUser(r *http.Request) (store.User, bool) {
	p, ok := s.currentUser(r)
	if !ok {
		return store.User{}, false
	}
	return p.User, true
}

func (s *Server) requireUser(w http.ResponseWriter, r *http.Request) (store.User, bool) {
	p, ok := s.currentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return store.User{}, false
	}
	return p.User, true
}

// requireScope requires a signed-in user; cookie grants all scopes, Bearer must include scope.
func (s *Server) requireScope(w http.ResponseWriter, r *http.Request, scope string) (store.User, bool) {
	p, ok := s.currentUser(r)
	if !ok {
		writeJSONError(w, http.StatusUnauthorized, "Not signed in")
		return store.User{}, false
	}
	if !p.hasScope(scope) {
		msg := "Missing required scope: " + scope
		if scope == store.ScopeAdmin {
			msg = "Missing required scope: admin (sign in with discloud auth login, or use a PAT that includes the admin scope)"
		}
		writeJSONError(w, http.StatusForbidden, msg)
		return store.User{}, false
	}
	if scope == store.ScopeAdmin && p.User.Role != store.RoleAdmin {
		writeJSONError(w, http.StatusForbidden, "Admin role required")
		return store.User{}, false
	}
	return p.User, true
}

// allowUploadAuth permits anonymous access, but rejects Bearer without upload scope
// and rejects invalid Bearer credentials (so they cannot silently fall back to anon).
func (s *Server) allowUploadAuth(w http.ResponseWriter, r *http.Request) bool {
	raw := bearerRaw(r)
	if raw == "" {
		return true
	}
	p, ok := s.currentUser(r)
	if !ok || !p.ViaBearer {
		writeJSONError(w, http.StatusUnauthorized, "Invalid or expired token")
		return false
	}
	if !p.hasScope(store.ScopeUpload) {
		writeJSONError(w, http.StatusForbidden, "Missing required scope: upload")
		return false
	}
	return true
}

// fileAccess is the result of authorizeFileAccess.
type fileAccess struct {
	File     store.File
	User     *store.User
	ViaToken bool
	Manager  bool // owner or admin — skip password + download cap
}

var (
	errPasswordRequired = errors.New("password required")
	errPasswordInvalid  = errors.New("password invalid")
)

// authorizeFileAccess is the single gate for all file-read paths.
// Visibility/token denials are uniform 404 (ErrNotFound). Invalid ids are errInvalidID (400).
// Password failures are errPasswordRequired / errPasswordInvalid (401).
func (s *Server) authorizeFileAccess(r *http.Request, id string) (fileAccess, error) {
	parsed, err := parseID(id)
	if err != nil {
		return fileAccess{}, errInvalidID
	}
	f, err := s.store.GetFile(r.Context(), parsed)
	if err != nil {
		return fileAccess{}, err
	}
	now := s.now().UTC()
	if !f.ExpiresAt.After(now) {
		return fileAccess{}, store.ErrNotFound
	}

	var userPtr *store.User
	if p, ok := s.currentUser(r); ok {
		// Cookie → full access. Bearer needs read to use owner/admin identity on get/inspect.
		if !p.ViaBearer || p.hasScope(store.ScopeRead) {
			u := p.User
			userPtr = &u
		}
	}

	access := fileAccess{File: f, User: userPtr}
	allowed := false

	if f.Visibility == store.VisibilityPublic || f.Visibility == "" {
		allowed = true
	}
	if userPtr != nil {
		if userPtr.Role == store.RoleAdmin {
			allowed = true
			access.Manager = true
		} else if f.OwnerUserID != nil && *f.OwnerUserID == userPtr.ID {
			allowed = true
			access.Manager = true
		}
	}
	if !allowed {
		raw := r.URL.Query().Get("token")
		if raw == "" {
			raw = r.Header.Get("X-File-Token")
		}
		if raw != "" && f.AccessTokenHash != "" && s.keys.FileTokenMatch(raw, f.AccessTokenHash) {
			allowed = true
			access.ViaToken = true
		}
	}
	if !allowed {
		return fileAccess{}, store.ErrNotFound
	}

	if f.PasswordHash != "" && !access.Manager {
		if err := s.checkFilePassword(r, f); err != nil {
			return fileAccess{}, err
		}
	}
	return access, nil
}

func (s *Server) checkFilePassword(r *http.Request, f store.File) error {
	if pw := r.Header.Get(filePasswordHeader); pw != "" {
		if auth.VerifyPassword(f.PasswordHash, pw) {
			return nil
		}
		return errPasswordInvalid
	}
	if c, err := r.Cookie(fileUnlockCookieName(f.ID)); err == nil && c.Value != "" {
		if s.keys.FileUnlockMatch(f.ID, c.Value, s.now().UTC().Unix()) {
			return nil
		}
	}
	return errPasswordRequired
}

func fileUnlockCookieName(fileID string) string {
	return "discloud_unlock_" + fileID
}

func (s *Server) canManageFile(u store.User, f store.File) bool {
	if u.Role == store.RoleAdmin {
		return true
	}
	return f.OwnerUserID != nil && *f.OwnerUserID == u.ID
}

func (s *Server) handleVisibility(w http.ResponseWriter, r *http.Request) {
	u, ok := s.requireScope(w, r, store.ScopeManage)
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
	u, ok := s.requireScope(w, r, store.ScopeManage)
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
	u, ok := s.requireScope(w, r, store.ScopeManage)
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
	if err := s.store.DeleteFile(r.Context(), id); err != nil {
		s.log.Error("delete file failed", "id", id, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.log.Info("delete file", "file", id, "user", u.ID)
	w.WriteHeader(http.StatusNoContent)
}

// RunCleanup deletes expired files and expires abandoned upload sessions
// periodically until ctx is cancelled.
func (s *Server) RunCleanup(ctx context.Context) {
	run := func() {
		now := s.now().UTC()
		n, err := s.store.DeleteExpiredFiles(ctx, now, cleanupBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("cleanup failed", "error", err)
		} else if n > 0 {
			s.log.Info("cleanup", "deleted", n)
		}
		un, err := s.store.ExpireUploadSessions(ctx, now, cleanupBatchSize)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("upload session expire failed", "error", err)
		} else if un > 0 {
			s.log.Info("upload sessions expired", "count", un)
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
