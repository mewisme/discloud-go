// Package server implements the discloud HTTP API.
package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

// Store is the metadata persistence the handlers need; *store.Store satisfies
// it, tests use an in-memory implementation.
type Store interface {
	CreateFile(ctx context.Context, f store.File) error
	GetFile(ctx context.Context, id string) (store.File, error)
	ListFiles(ctx context.Context, limit int) ([]store.File, error)
	HasChunk(ctx context.Context, hash string) (bool, error)
	PutChunk(ctx context.Context, c store.Chunk) error
	GetChunks(ctx context.Context, hashes []string) (map[string]store.Chunk, error)
	DeleteChunksByMessageID(ctx context.Context, messageID string) error
	EnsureBots(ctx context.Context, count int) error
	Ping(ctx context.Context) error
}

// Cache caches signed CDN URLs per Discord message id.
type Cache interface {
	GetURL(ctx context.Context, messageID string) (string, bool)
	SetURL(ctx context.Context, messageID, cdnURL string)
	Ping(ctx context.Context) error
}

type Server struct {
	log           *slog.Logger
	store         Store
	cache         Cache
	discord       *discord.Client
	publicBaseURL string
	// cdn streams chunk bytes from the Discord CDN; no overall timeout so
	// slow client downloads aren't cut off mid-stream.
	cdn *http.Client
}

func New(log *slog.Logger, st Store, ca Cache, dc *discord.Client, publicBaseURL string) *Server {
	return &Server{
		log:           log,
		store:         st,
		cache:         ca,
		discord:       dc,
		publicBaseURL: publicBaseURL,
		cdn:           &http.Client{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("GET /api/chunks/{hash}", s.handleChunkCheck)
	mux.HandleFunc("POST /api/chunks", s.handleChunkUpload)
	mux.HandleFunc("POST /api/upload/complete", s.handleUploadComplete)
	mux.HandleFunc("GET /api/files", s.handleListFiles)
	mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	mux.HandleFunc("GET /api/info", s.handleInfo)
	mux.HandleFunc("GET /f/{id}", s.handleDownload)
	mux.HandleFunc("GET /f/{id}/{name...}", s.handleDownload)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	return s.withLogging(withCORS(mux))
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

// handleInfo is a tiny public config the web client uses to size upload
// parallelism (one worker per Discord bot when multiple tokens are set).
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	bots := s.discord.TokenCount()
	workers := singleBotUploadConcurrency
	if bots > 1 {
		workers = bots
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"bots":      bots,
		"chunkSize": chunkSize,
		"workers":   workers,
	})
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

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
