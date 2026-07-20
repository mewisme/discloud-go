package server

// Chunked upload flow for proxies that cap request body size (e.g. Cloudflare
// limits proxied uploads to 100 MB): the client splits the file into
// chunkSize pieces, checks each piece's SHA-256 against the content-addressed
// chunk store, uploads only missing pieces, and finally assembles the file
// from the ordered hash list. Retried uploads skip chunks that already exist.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/mewisme/discloud-go/internal/discord"
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

	out, err := s.storeChunks(r.Context(), [][]byte{data})
	if err != nil {
		s.log.Error("chunk upload failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Upload failed")
		return
	}
	writeJSON(w, http.StatusOK, out[0])
}

// handleChunkBatch stores up to discord.MaxAttachments chunks in one Discord
// message when they are new. Multipart field names: files[0]…files[n-1].
func (s *Server) handleChunkBatch(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, int64(discord.MaxAttachments)*chunkSize+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid multipart body")
		return
	}
	var blobs [][]byte
	for i := 0; i < discord.MaxAttachments; i++ {
		file, _, err := r.FormFile(fmt.Sprintf("files[%d]", i))
		if err != nil {
			break
		}
		data, err := io.ReadAll(io.LimitReader(file, chunkSize+1))
		_ = file.Close()
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "Failed to read chunk")
			return
		}
		if len(data) == 0 || len(data) > chunkSize {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("Each chunk must be 1..%d bytes", chunkSize))
			return
		}
		blobs = append(blobs, data)
	}
	if len(blobs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "No chunks in request")
		return
	}

	out, err := s.storeChunks(r.Context(), blobs)
	if err != nil {
		s.log.Error("chunk batch failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Upload failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"chunks": out})
}

type chunkUploadResult struct {
	Hash    string `json:"hash"`
	Existed bool   `json:"existed"`
}

// storeChunks content-addresses blobs, uploading only missing ones to Discord
// in a single multi-attachment message when possible.
func (s *Server) storeChunks(ctx context.Context, blobs [][]byte) ([]chunkUploadResult, error) {
	out := make([]chunkUploadResult, len(blobs))
	type miss struct {
		i    int
		hash string
		data []byte
	}
	var missing []miss

	for i, data := range blobs {
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		exists, err := s.store.HasChunk(ctx, hash)
		if err != nil {
			return nil, err
		}
		if exists {
			out[i] = chunkUploadResult{Hash: hash, Existed: true}
			continue
		}
		missing = append(missing, miss{i: i, hash: hash, data: data})
	}

	if len(missing) == 0 {
		return out, nil
	}

	parts := make([]discord.Part, len(missing))
	for j, m := range missing {
		parts[j] = discord.Part{Name: "chunk-" + m.hash[:16], Data: m.data}
	}
	refs, err := s.discord.UploadParts(ctx, parts)
	if err != nil {
		return nil, err
	}
	for j, m := range missing {
		if err := s.store.PutChunk(ctx, store.Chunk{
			Hash: m.hash, MessageID: refs[j], Size: int64(len(m.data)),
		}); err != nil {
			return nil, err
		}
		out[m.i] = chunkUploadResult{Hash: m.hash, Existed: false}
	}
	return out, nil
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
		messageIDs[i] = c.MessageID
	}

	f := store.File{
		ID:         newID(),
		Name:       formatFileName(req.FileName),
		Size:       fileSize,
		ChunkSize:  chunkSize,
		CreatedAt:  time.Now().UTC(),
		MessageIDs: messageIDs,
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		s.log.Error("persist file failed", "file", f.Name, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist file metadata")
		return
	}

	base := s.baseURL(r)
	s.log.Info("file assembled", "file", f.Name, "size", humanBytes(fileSize), "chunks", len(messageIDs))
	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":          f.ID,
		"fileName":        f.Name,
		"fileSize":        fileSize,
		"url":             fmt.Sprintf("%s/f/%s", base, f.ID),
		"longURL":         fmt.Sprintf("%s/f/%s/%s", base, f.ID, f.Name),
		"downloadURL":     fmt.Sprintf("%s/f/%s?download=1", base, f.ID),
		"longDownloadURL": fmt.Sprintf("%s/f/%s/%s?download=1", base, f.ID, f.Name),
	})
}
