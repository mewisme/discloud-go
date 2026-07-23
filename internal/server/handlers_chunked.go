package server

// Chunked upload flow for proxies that cap request body size (e.g. Cloudflare
// limits proxied uploads to 100 MB): the client splits the file into
// chunkSize pieces, checks each piece's SHA-256 against the content-addressed
// chunk store, uploads only missing pieces, and finally assembles the file
// from the ordered hash list. Retried uploads skip chunks that already exist.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/mewisme/discloud-go/internal/store"
)

var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// handleChunkCheck reports whether a chunk is already stored, so clients can
// skip re-uploading it.
func (s *Server) handleChunkCheck(w http.ResponseWriter, r *http.Request) {
	if !s.allowUploadAuth(w, r) {
		return
	}
	hash := r.PathValue("hash")
	if !hashPattern.MatchString(hash) {
		writeJSONError(w, http.StatusBadRequest, "Invalid chunk hash")
		return
	}
	exists, err := s.store.HasChunk(r.Context(), hash)
	if err != nil {
		s.log.Error("chunk check failed", "hash", hash, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if !exists {
		writeJSONError(w, http.StatusNotFound, "Chunk not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"exists": true})
}

// handleChunkUpload stores one chunk (at most chunkSize bytes). The hash is
// computed server-side from the received bytes, never trusted from the client.
func (s *Server) handleChunkUpload(w http.ResponseWriter, r *http.Request) {
	if !s.allowUploadAuth(w, r) {
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, chunkSize))
	if err != nil {
		writeJSONError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Chunk must be at most %d bytes", chunkSize))
		return
	}
	if len(data) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Empty chunk")
		return
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	exists, err := s.store.HasChunk(r.Context(), hash)
	if err != nil {
		s.log.Error("chunk lookup failed", "hash", hash, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if exists {
		writeJSON(w, http.StatusOK, map[string]any{"hash": hash, "existed": true})
		return
	}

	msgID, err := s.discord.UploadChunk(r.Context(), "chunk-"+hash[:16], data)
	if err != nil {
		s.log.Error("chunk upload failed", "hash", hash, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Upload failed")
		return
	}
	if err := s.store.PutChunk(r.Context(), store.Chunk{
		Hash: hash, MessageID: msgID.MessageID, BotID: msgID.BotID, Size: int64(len(data)),
	}); err != nil {
		s.log.Error("chunk persist failed", "hash", hash, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist chunk")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hash": hash, "existed": false})
}

// handleUploadComplete assembles a file from previously uploaded chunks.
func (s *Server) handleUploadComplete(w http.ResponseWriter, r *http.Request) {
	if !s.allowUploadAuth(w, r) {
		return
	}
	var req struct {
		FileName    string   `json:"fileName"`
		ChunkHashes []string `json:"chunkHashes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.FileName == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing fileName")
		return
	}
	if len(req.ChunkHashes) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Missing chunkHashes")
		return
	}
	if len(req.ChunkHashes) > maxChunksPerFile {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("Too many chunks (max %d)", maxChunksPerFile))
		return
	}
	for _, h := range req.ChunkHashes {
		if !hashPattern.MatchString(h) {
			writeJSONError(w, http.StatusBadRequest, "Invalid chunk hash: "+h)
			return
		}
	}

	f, rawToken, err := s.assembleFileFromHashes(r, req.FileName, req.ChunkHashes)
	if err != nil {
		var httpErr *uploadHTTPError
		if errors.As(err, &httpErr) {
			writeJSONError(w, httpErr.code, httpErr.msg)
			return
		}
		s.log.Error("assemble failed", "file", req.FileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to assemble file")
		return
	}
	s.log.Info("file assembled", "file", f.Name, "size", humanBytes(f.Size), "chunks", len(req.ChunkHashes))
	s.writeFileCreated(w, r, f, rawToken)
}
