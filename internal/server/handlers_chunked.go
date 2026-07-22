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

	known, err := s.store.GetChunks(r.Context(), req.ChunkHashes)
	if err != nil {
		s.log.Error("chunk resolve failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Range math requires every chunk except the last to be exactly chunkSize.
	var fileSize int64
	parts := make([]store.FilePart, len(req.ChunkHashes))
	messageIDs := make([]string, len(req.ChunkHashes))
	for i, h := range req.ChunkHashes {
		c, ok := known[h]
		if !ok {
			writeJSONError(w, http.StatusBadRequest, "Unknown chunk hash: "+h)
			return
		}
		if i < len(req.ChunkHashes)-1 && c.Size != chunkSize {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("Chunk %d is %d bytes; every chunk except the last must be exactly %d bytes", i, c.Size, chunkSize))
			return
		}
		fileSize += c.Size
		parts[i] = store.FilePart{MessageID: c.MessageID, BotID: c.BotID}
		messageIDs[i] = c.MessageID
	}

	name := formatFileName(req.FileName)
	// Same logged-in user + same name + same chunks → reuse. Other users ignored.
	if u, ok := s.sessionUser(r); ok {
		uid := u.ID
		existing, err := s.store.FindFileByNameAndParts(r.Context(), &uid, name, messageIDs, s.now().UTC())
		if err == nil {
			if existing.Status != store.FileStatusDuplicate {
				if err := s.store.UpdateFileStatus(r.Context(), existing.ID, store.FileStatusDuplicate); err != nil {
					s.log.Error("mark duplicate failed", "file", existing.ID, "error", err)
					writeJSONError(w, http.StatusInternalServerError, "Internal server error")
					return
				}
				existing.Status = store.FileStatusDuplicate
			}
			s.log.Info("file reused", "file", existing.Name, "id", existing.ID, "chunks", len(parts))
			s.writeFileCreated(w, r, existing, "")
			return
		}
		if !errors.Is(err, store.ErrNotFound) {
			s.log.Error("file dedupe lookup failed", "file", name, "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
	}

	f, rawToken, err := s.newOwnedFile(r, newID(), name, fileSize, parts)
	if err != nil {
		s.log.Error("prepare file failed", "file", req.FileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to prepare file metadata")
		return
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		s.log.Error("persist file failed", "file", f.Name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist file metadata")
		return
	}

	s.log.Info("file assembled", "file", f.Name, "size", humanBytes(fileSize), "chunks", len(parts))
	s.writeFileCreated(w, r, f, rawToken)
}
