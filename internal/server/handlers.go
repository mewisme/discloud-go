package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/store"
)

const (
	chunkSize = 8 << 20 // 8 MB, Discord's attachment limit for bots without nitro boosts
	// rangeWindow caps open-ended Range requests, mirroring the original's
	// 5 MB windows for media seeking.
	rangeWindow = 5 << 20
	// singleBotUploadConcurrency is the in-flight Discord POST limit when only
	// one bot token is configured (pipeline a few chunks despite one rate clock).
	singleBotUploadConcurrency = 3
	listLimit                  = 50
)

// discordUploadLimit is how many Discord attachment uploads may run at once.
// With multiple bot tokens, match the token count so each bot can work in
// parallel; with one token, keep a small pipeline.
func (s *Server) discordUploadLimit() int {
	n := s.discord.TokenCount()
	if n <= 1 {
		return singleBotUploadConcurrency
	}
	return n
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	rawName := r.URL.Query().Get("fileName")
	if rawName == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing fileName query param")
		return
	}
	fileName := formatFileName(rawName)
	fileID := newID()

	type item struct {
		idx  int
		data []byte
	}
	var (
		mu         sync.Mutex
		messageIDs []string
		batch      []item
	)
	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(s.discordUploadLimit())

	flush := func(items []item) {
		if len(items) == 0 {
			return
		}
		g.Go(func() error {
			parts := make([]discord.Part, len(items))
			for i, it := range items {
				parts[i] = discord.Part{
					Name: fmt.Sprintf("%s-chunk-%d", fileName, it.idx+1),
					Data: it.data,
				}
			}
			refs, err := s.discord.UploadParts(ctx, parts)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for i, it := range items {
				for len(messageIDs) <= it.idx {
					messageIDs = append(messageIDs, "")
				}
				messageIDs[it.idx] = refs[i]
			}
			return nil
		})
	}

	fileSize, err := forEachChunk(r.Body, chunkSize, func(idx int, data []byte) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Own the slice; forEachChunk allocates a fresh buffer each call.
		cp := make([]byte, len(data))
		copy(cp, data)
		batch = append(batch, item{idx: idx, data: cp})
		if len(batch) >= discord.MaxAttachments {
			flush(batch)
			batch = nil
		}
		return nil
	})
	flush(batch)
	if uploadErr := g.Wait(); err == nil {
		err = uploadErr
	}
	if err != nil {
		s.log.Error("upload failed", "file", fileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Upload failed")
		return
	}
	if fileSize == 0 {
		writeJSONError(w, http.StatusBadRequest, "Empty request body")
		return
	}

	f := store.File{
		ID:         fileID,
		Name:       fileName,
		Size:       fileSize,
		ChunkSize:  chunkSize,
		CreatedAt:  time.Now().UTC(),
		MessageIDs: messageIDs,
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		s.log.Error("persist file failed", "file", fileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist file metadata")
		return
	}

	base := s.baseURL(r)
	s.log.Info("file uploaded", "file", fileName, "size", humanBytes(fileSize), "chunks", len(messageIDs))
	writeJSON(w, http.StatusOK, map[string]any{
		"fileId":          fileID,
		"fileName":        fileName,
		"fileSize":        fileSize,
		"url":             fmt.Sprintf("%s/f/%s", base, fileID),
		"longURL":         fmt.Sprintf("%s/f/%s/%s", base, fileID, fileName),
		"downloadURL":     fmt.Sprintf("%s/f/%s?download=1", base, fileID),
		"longDownloadURL": fmt.Sprintf("%s/f/%s/%s?download=1", base, fileID, fileName),
	})
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	f, err := s.store.GetFile(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "Cannot find the specified file", http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("get file failed", "id", r.PathValue("id"), "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	ct := mime.TypeByExtension(filepath.Ext(f.Name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, f.Name))

	full := byteRange{start: 0, end: f.Size - 1}
	status := http.StatusOK
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		full, err = parseRange(rangeHeader, f.Size, rangeWindow)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", f.Size))
			http.Error(w, "Requested range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", full.start, full.end, f.Size))
	}
	w.Header().Set("Content-Length", strconv.FormatInt(full.end-full.start+1, 10))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}

	for _, span := range partsForRange(f.ChunkSize, full) {
		if span.idx >= len(f.MessageIDs) {
			break // metadata/size mismatch; stop rather than 500 mid-stream
		}
		if err := s.streamPart(w, r, f.MessageIDs[span.idx], span); err != nil {
			// Headers are already sent; all we can do is stop and log.
			s.log.Error("stream part failed", "file", f.Name, "part", span.idx, "error", err)
			return
		}
	}
}

// streamPart resolves the chunk's signed CDN URL (cache first) and copies the
// requested byte span to the client.
func (s *Server) streamPart(w http.ResponseWriter, r *http.Request, messageID string, span partSpan) error {
	ctx := r.Context()
	cdnURL, ok := s.cache.GetURL(ctx, messageID)
	if !ok {
		var err error
		cdnURL, err = s.discord.AttachmentURL(ctx, messageID)
		if err != nil {
			return err
		}
		s.cache.SetURL(ctx, messageID, cdnURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cdnURL, nil)
	if err != nil {
		return err
	}
	partial := span.start > 0 || span.end < chunkSize-1
	if partial {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", span.start, span.end))
	}
	resp, err := s.cdn.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		_, err = io.Copy(w, resp.Body)
	case http.StatusOK:
		// CDN ignored the Range header; slice the span out of the full body.
		if span.start > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, span.start); err != nil {
				return err
			}
		}
		_, err = io.CopyN(w, resp.Body, span.end-span.start+1)
		if errors.Is(err, io.EOF) {
			err = nil // last chunk is usually shorter than chunkSize
		}
	default:
		return fmt.Errorf("cdn responded %s", resp.Status)
	}
	return err
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.store.ListFiles(r.Context(), listLimit)
	if err != nil {
		s.log.Error("list files failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list files")
		return
	}
	if files == nil {
		files = []store.File{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	f, err := s.store.GetFile(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) baseURL(r *http.Request) string {
	if s.publicBaseURL != "" {
		return strings.TrimSuffix(s.publicBaseURL, "/")
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func newID() string {
	b := make([]byte, 16)
	rand.Read(b) // never fails per crypto/rand docs
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
