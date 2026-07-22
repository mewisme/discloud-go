package server

import (
	"context"
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

	var (
		mu    sync.Mutex
		parts []store.FilePart
	)
	g, ctx := errgroup.WithContext(r.Context())
	g.SetLimit(s.discordUploadLimit())

	fileSize, err := forEachChunk(r.Body, chunkSize, func(idx int, data []byte) error {
		if ctx.Err() != nil { // an upload already failed; stop reading
			return ctx.Err()
		}
		g.Go(func() error {
			up, err := s.discord.UploadChunk(ctx, fmt.Sprintf("%s-chunk-%d", fileName, idx+1), data)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for len(parts) <= idx {
				parts = append(parts, store.FilePart{BotID: -1})
			}
			parts[idx] = store.FilePart{MessageID: up.MessageID, BotID: up.BotID}
			return nil
		})
		return nil
	})
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
		ID:        fileID,
		Name:      fileName,
		Size:      fileSize,
		ChunkSize: chunkSize,
		CreatedAt: time.Now().UTC(),
		Parts:     parts,
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		s.log.Error("persist file failed", "file", fileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist file metadata")
		return
	}

	base := s.baseURL(r)
	s.log.Info("file uploaded", "file", fileName, "size", humanBytes(fileSize), "chunks", len(parts))
	writeJSON(w, http.StatusOK, fileLinks(base, fileID, fileName, fileSize))
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	f, err := s.store.GetFile(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrNotFound) {
		if r.URL.Query().Get("json") == "1" {
			writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
			return
		}
		http.Error(w, "Cannot find the specified file", http.StatusNotFound)
		return
	}
	if err != nil {
		s.log.Error("get file failed", "id", r.PathValue("id"), "error", err)
		if r.URL.Query().Get("json") == "1" {
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("json") == "1" {
		writeJSON(w, http.StatusOK, f)
		return
	}

	// Single-chunk + inline: 302 to Discord CDN so file bytes skip our egress.
	// Multi-chunk still needs proxy stitching; ?download=1 keeps Content-Disposition.
	if r.URL.Query().Get("download") != "1" && len(f.Parts) == 1 {
		cdnURL, err := s.resolveCDNURL(r.Context(), f.Parts[0])
		if err != nil {
			s.log.Error("resolve cdn failed", "file", f.Name, "error", err)
			http.Error(w, "Chunk unavailable; re-upload the file", http.StatusBadGateway)
			return
		}
		if r.Method != http.MethodHead {
			served := f.Size
			if rh := r.Header.Get("Range"); rh != "" {
				if br, err := parseRange(rh, f.Size, rangeWindow); err == nil {
					served = br.end - br.start + 1
				}
			}
			s.trackAsync(f.ID, accessKind(r), served, r)
		}
		http.Redirect(w, r, cdnURL, http.StatusFound)
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

	spans := partsForRange(f.ChunkSize, full)
	if len(spans) == 0 || spans[0].idx >= len(f.Parts) {
		http.Error(w, "File has no readable chunks", http.StatusInternalServerError)
		return
	}
	// Resolve the first part before committing headers so a dead Discord
	// attachment becomes a real error instead of a truncated 200 body.
	if _, err := s.resolveCDNURL(r.Context(), f.Parts[spans[0].idx]); err != nil {
		s.log.Error("stream part failed", "file", f.Name, "part", spans[0].idx, "error", err)
		http.Error(w, "Chunk unavailable; re-upload the file", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Length", strconv.FormatInt(full.end-full.start+1, 10))
	w.WriteHeader(status)

	served := full.end - full.start + 1
	if r.Method != http.MethodHead {
		s.trackAsync(f.ID, accessKind(r), served, r)
	} else {
		return
	}

	for _, span := range spans {
		if span.idx >= len(f.Parts) {
			break // metadata/size mismatch; stop rather than 500 mid-stream
		}
		if err := s.streamPart(w, r, f.Parts[span.idx], span); err != nil {
			// Headers are already sent; all we can do is stop and log.
			s.log.Error("stream part failed", "file", f.Name, "part", span.idx, "error", err)
			return
		}
	}
}

// resolveCDNURL returns a signed Discord CDN URL for a part, caching hits.
// When Discord no longer has the attachment, the content-addressed chunk row
// is dropped so the next upload re-POSTs those bytes.
func (s *Server) resolveCDNURL(ctx context.Context, part store.FilePart) (string, error) {
	cdnURL, ok := s.cache.GetURL(ctx, part.MessageID)
	if ok {
		return cdnURL, nil
	}
	cdnURL, err := s.discord.AttachmentURL(ctx, part.MessageID, part.BotID)
	if err != nil {
		if delErr := s.store.DeleteChunksByMessageID(ctx, part.MessageID); delErr != nil {
			s.log.Error("purge dead chunk failed", "message", part.MessageID, "error", delErr)
		} else {
			s.log.Info("purged dead chunk refs", "message", part.MessageID)
		}
		return "", err
	}
	s.cache.SetURL(ctx, part.MessageID, cdnURL)
	return cdnURL, nil
}

// streamPart resolves the chunk's signed CDN URL (cache first) and copies the
// requested byte span to the client.
func (s *Server) streamPart(w http.ResponseWriter, r *http.Request, part store.FilePart, span partSpan) error {
	cdnURL, err := s.resolveCDNURL(r.Context(), part)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, cdnURL, nil)
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
	if u := strings.TrimSuffix(strings.TrimSpace(s.publicBaseURL), "/"); u != "" {
		return u
	}
	scheme := "http"
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// fileLinks builds share/download URLs from API_URL (or request host).
func fileLinks(base, fileID, fileName string, fileSize int64) map[string]any {
	return map[string]any{
		"fileId":          fileID,
		"fileName":        fileName,
		"fileSize":        fileSize,
		"url":             fmt.Sprintf("%s/f/%s", base, fileID),
		"longURL":         fmt.Sprintf("%s/f/%s/%s", base, fileID, fileName),
		"downloadURL":     fmt.Sprintf("%s/f/%s?download=1", base, fileID),
		"longDownloadURL": fmt.Sprintf("%s/f/%s/%s?download=1", base, fileID, fileName),
	}
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
