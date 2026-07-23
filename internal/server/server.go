// Package server implements the discloud HTTP API.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

// Store is the metadata persistence the handlers need; *store.Store satisfies
// it, tests use an in-memory implementation.
type Store interface {
	CreateFile(ctx context.Context, f store.File) error
	GetFile(ctx context.Context, id string) (store.File, error)
	FindFileByNameAndParts(ctx context.Context, ownerUserID *string, name string, messageIDs []string, now time.Time) (store.File, error)
	UpdateFileStatus(ctx context.Context, id, status string) error
	ListFilesByOwner(ctx context.Context, ownerID string, limit, offset int) ([]store.File, error)
	HasChunk(ctx context.Context, hash string) (bool, error)
	PutChunk(ctx context.Context, c store.Chunk) error
	GetChunks(ctx context.Context, hashes []string) (map[string]store.Chunk, error)
	DeleteChunksByMessageID(ctx context.Context, messageID string) error
	EnsureBots(ctx context.Context, count int) error
	RecordEvent(ctx context.Context, e store.Event) error
	GetFileInspect(ctx context.Context, id string) (store.FileInspect, error)
	CreateUser(ctx context.Context, id, username, passwordHash string) (store.User, error)
	GetUserByUsername(ctx context.Context, username string) (store.User, error)
	GetUserByID(ctx context.Context, id string) (store.User, error)
	CreateSession(ctx context.Context, sess store.Session) error
	GetUserBySessionHash(ctx context.Context, tokenHash string, now time.Time) (store.User, error)
	GetSessionByTokenHash(ctx context.Context, tokenHash string, now time.Time) (store.Session, error)
	TouchSession(ctx context.Context, tokenHash, ip, userAgent string, now time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteSessionsByUserID(ctx context.Context, userID string) error
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
	UpdateDefaultVisibility(ctx context.Context, userID, visibility string) error
	OwnerFileStats(ctx context.Context, ownerID string, now time.Time, soonWithin time.Duration) (store.OwnerStats, error)
	UpdateVisibility(ctx context.Context, id, visibility string, tokenHash *string, rotatedAt *time.Time) error
	RotateAccessToken(ctx context.Context, id, tokenHash string, rotatedAt time.Time) error
	DeleteFile(ctx context.Context, id string) error
	DeleteExpiredFiles(ctx context.Context, now time.Time, limit int) (int64, error)
	ExtendExpiration(ctx context.Context, id string, now time.Time, ext, capDur time.Duration) (time.Time, error)
	CreateUploadSession(ctx context.Context, u store.UploadSession) error
	GetUploadSession(ctx context.Context, id string) (store.UploadSession, error)
	CountOpenUploadSessionsByOwner(ctx context.Context, ownerUserID string) (int64, error)
	CountOpenUploadSessionsAnon(ctx context.Context, fingerprintPrefix string) (int64, error)
	SumOpenUploadBytesByOwner(ctx context.Context, ownerUserID string) (int64, error)
	RegisterUploadPart(ctx context.Context, uploadID string, idx int, hash string, now time.Time) error
	BeginUploadComplete(ctx context.Context, uploadID string, now time.Time) (store.UploadSession, error)
	FinishUploadComplete(ctx context.Context, uploadID, fileID string, now time.Time) error
	AbortUploadComplete(ctx context.Context, uploadID string, now time.Time) error
	CancelUploadSession(ctx context.Context, uploadID string, now time.Time) error
	ExpireUploadSessions(ctx context.Context, now time.Time, limit int) (int64, error)
	UpdateFileShare(ctx context.Context, id string, p store.FileSharePatch) error
	IncrementDownloadCount(ctx context.Context, id string) error
	RevokeFile(ctx context.Context, id string, now time.Time) error
	CreateAPIToken(ctx context.Context, t store.APIToken) error
	GetAPITokenByHash(ctx context.Context, tokenHash string) (store.APIToken, error)
	ListAPITokensByUser(ctx context.Context, userID string) ([]store.APIToken, error)
	RevokeAPIToken(ctx context.Context, id, userID string, now time.Time) error
	RevokeAPITokensByUser(ctx context.Context, userID string, now time.Time) error
	TouchAPITokenLastUsed(ctx context.Context, id string, now time.Time) error
	CountAPITokensByUser(ctx context.Context, userID string) (int64, error)
	Ping(ctx context.Context) error
}

// Cache caches signed CDN URLs and supports rate-limit counters.
type Cache interface {
	GetURL(ctx context.Context, messageID string) (string, bool)
	SetURL(ctx context.Context, messageID, cdnURL string)
	Incr(ctx context.Context, key string) (int64, error)
	IncrBy(ctx context.Context, key string, n int64) (int64, error)
	Expire(ctx context.Context, key string, ttl time.Duration) error
	Ping(ctx context.Context) error
}

// Options configures the HTTP server beyond store/cache/discord.
type Options struct {
	PublicBaseURL string
	VisitorSalt   string
	WebOrigin     string
	CookieSecure  bool
	// TrustProxy honors X-Forwarded-For / X-Real-IP. Only enable when a
	// trusted edge strips client-supplied forwarding headers.
	TrustProxy bool
	Keys       auth.Keys
	Now        func() time.Time // nil → time.Now

	RateLimitUploadPerMin   int
	RateLimitDownloadPerMin int
	MaxUserBytes            int64
	MaxAnonUploadsPerDay    int
	MaxAnonBytesPerDay      int64
	MaxRawUploadBytes       int64
	CaptchaSecret           string
}

type Server struct {
	log           *slog.Logger
	store         Store
	cache         Cache
	discord       *discord.Client
	publicBaseURL string
	visitorSalt   string
	webOrigin     string
	cookieSecure  bool
	trustProxy    bool
	keys          auth.Keys
	now           func() time.Time
	// cdn streams chunk bytes from the Discord CDN. ResponseHeaderTimeout
	// bounds hung connects; no Client.Timeout so slow body reads continue.
	cdn *http.Client

	rateLimitUploadPerMin   int
	rateLimitDownloadPerMin int
	maxUserBytes            int64
	maxAnonUploadsPerDay    int
	maxAnonBytesPerDay      int64
	maxRawUploadBytes       int64
	captchaSecret           string

	// captchaVerify is set in tests; nil → Turnstile siteverify.
	captchaVerify func(ctx context.Context, secret, token, ip string) error
}

func New(log *slog.Logger, st Store, ca Cache, dc *discord.Client, opts Options) *Server {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	maxRaw := opts.MaxRawUploadBytes
	if maxRaw <= 0 {
		maxRaw = int64(maxChunksPerFile) * chunkSize
	}
	return &Server{
		log:                     log,
		store:                   st,
		cache:                   ca,
		discord:                 dc,
		publicBaseURL:           opts.PublicBaseURL,
		visitorSalt:             opts.VisitorSalt,
		webOrigin:               opts.WebOrigin,
		cookieSecure:            opts.CookieSecure,
		trustProxy:              opts.TrustProxy,
		keys:                    opts.Keys,
		now:                     now,
		rateLimitUploadPerMin:   opts.RateLimitUploadPerMin,
		rateLimitDownloadPerMin: opts.RateLimitDownloadPerMin,
		maxUserBytes:            opts.MaxUserBytes,
		maxAnonUploadsPerDay:    opts.MaxAnonUploadsPerDay,
		maxAnonBytesPerDay:      opts.MaxAnonBytesPerDay,
		maxRawUploadBytes:       maxRaw,
		captchaSecret:           opts.CaptchaSecret,
		cdn: &http.Client{
			Transport: &http.Transport{
				ResponseHeaderTimeout: 2 * time.Minute,
			},
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/signup", s.handleSignup)
	mux.HandleFunc("POST /api/auth/signin", s.handleSignin)
	mux.HandleFunc("POST /api/auth/signout", s.handleSignout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)
	mux.HandleFunc("PATCH /api/auth/preferences", s.handleUpdatePreferences)
	mux.HandleFunc("POST /api/auth/password", s.handleChangePassword)
	mux.HandleFunc("POST /api/auth/tokens", s.handleCreateAPIToken)
	mux.HandleFunc("GET /api/auth/tokens", s.handleListAPITokens)
	mux.HandleFunc("DELETE /api/auth/tokens/{id}", s.handleRevokeAPIToken)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/chunks/{hash}", s.handleChunkCheck)
	mux.HandleFunc("POST /api/chunks", s.handleChunkUpload)
	mux.HandleFunc("POST /api/upload/complete", s.handleUploadComplete)
	mux.HandleFunc("POST /api/uploads", s.handleCreateUpload)
	mux.HandleFunc("GET /api/uploads/{id}", s.handleGetUpload)
	mux.HandleFunc("PUT /api/uploads/{id}/parts/{idx}", s.handleRegisterUploadPart)
	mux.HandleFunc("POST /api/uploads/{id}/parts", s.handleRegisterUploadParts)
	mux.HandleFunc("POST /api/uploads/{id}/complete", s.handleCompleteUploadSession)
	mux.HandleFunc("DELETE /api/uploads/{id}", s.handleCancelUpload)
	mux.HandleFunc("GET /api/files", s.handleListFiles)
	mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	mux.HandleFunc("GET /api/files/{id}/inspect", s.handleInspect)
	mux.HandleFunc("PATCH /api/files/{id}/visibility", s.handleVisibility)
	mux.HandleFunc("PATCH /api/files/{id}/share", s.handleShareSettings)
	mux.HandleFunc("POST /api/files/{id}/unlock", s.handleUnlockFile)
	mux.HandleFunc("POST /api/files/{id}/revoke", s.handleRevokeFile)
	mux.HandleFunc("POST /api/files/{id}/access-token/rotate", s.handleRotateToken)
	mux.HandleFunc("DELETE /api/files/{id}", s.handleDeleteFile)
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("GET /f/{id}", s.handleDownload)
	mux.HandleFunc("GET /f/{id}/{name...}", s.handleDownload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.HandleFunc("GET /install.sh", s.handleInstallSH)
	mux.HandleFunc("GET /install.ps1", s.handleInstallPS1)
	return s.withLogging(s.withCORSAndCSRF(mux))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		http.Error(w, "postgres unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.cache.Ping(ctx); err != nil {
		http.Error(w, "valkey unavailable", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}

// handleInfo is public upload sizing only — never expose bot/worker counts.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"chunkSize": chunkSize,
		"uploads": map[string]any{
			"sessions":    true,
			"maxFileSize": int64(maxChunksPerFile) * chunkSize,
		},
	}
	if s.captchaEnabled() {
		info["captcha"] = map[string]any{
			"required": true,
			"provider": "turnstile",
		}
	}
	writeJSON(w, http.StatusOK, info)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// redactQuery strips token from a query string for safe logging.
func redactQuery(rawQuery string) string {
	if rawQuery == "" || !strings.Contains(rawQuery, "token=") {
		return rawQuery
	}
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ""
	}
	if q.Has("token") {
		q.Set("token", "[redacted]")
	}
	return q.Encode()
}

func (s *Server) withCORSAndCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == s.webOrigin {
			w.Header().Set("Access-Control-Allow-Origin", s.webOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, Authorization, X-File-Token, X-File-Password, X-Upload-Token")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// CSRF: only mutating methods. GET/HEAD navigations often send a
		// session cookie with no Origin; requiring Origin there 403s public /f/{id}.
		// Bearer-only (no session cookie) skips Origin — PATs are not browser cookies.
		mutating := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		if mutating {
			_, cookieErr := r.Cookie(sessionCookieName)
			hasCookie := cookieErr == nil
			rawBearer := bearerRaw(r)
			bearerOnly := !hasCookie && strings.HasPrefix(rawBearer, "dc_")
			if !bearerOnly && (hasCookie || origin != "") && origin != s.webOrigin {
				writeJSONError(w, http.StatusForbidden, "Origin not allowed")
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}
