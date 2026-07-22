package server

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/mewisme/discloud-go/internal/auth"
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
	// Not exposed publicly — /api/info returns chunkSize only.
	singleBotUploadConcurrency = 3
	listLimit                  = 50
	// maxChunksPerFile caps assembled metadata (~64 GiB at chunkSize).
	maxChunksPerFile = 8192
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

	f, rawToken, err := s.newOwnedFile(r, fileID, fileName, fileSize, parts)
	if err != nil {
		s.log.Error("prepare file failed", "file", fileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to prepare file metadata")
		return
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		s.log.Error("persist file failed", "file", fileName, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to persist file metadata")
		return
	}

	s.log.Info("file uploaded", "file", fileName, "size", humanBytes(fileSize), "chunks", len(parts))
	s.writeFileCreated(w, r, f, rawToken)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	access, err := s.authorizeFileAccess(r, r.PathValue("id"))
	if errors.Is(err, errInvalidID) {
		if r.URL.Query().Get("json") == "1" {
			writeJSONError(w, http.StatusBadRequest, "Invalid file id")
			return
		}
		http.Error(w, "Invalid file id", http.StatusBadRequest)
		return
	}
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
	f := access.File
	if access.ViaToken {
		w.Header().Set("Referrer-Policy", "no-referrer")
	}

	if r.URL.Query().Get("json") == "1" {
		writeJSON(w, http.StatusOK, s.fileMetaDTO(r, f, "", access.User))
		return
	}

	isDownload := r.URL.Query().Get("download") == "1"
	wantExtend := isDownload && r.Method != http.MethodHead && r.Header.Get("Range") == ""

	// Single-chunk + inline: 302 to Discord CDN so file bytes skip our egress.
	if !isDownload && len(f.Parts) == 1 {
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

	// Single-chunk + download=1 (no Range): extend then 302.
	if isDownload && len(f.Parts) == 1 && r.Header.Get("Range") == "" {
		cdnURL, err := s.resolveCDNURL(r.Context(), f.Parts[0])
		if err != nil {
			s.log.Error("resolve cdn failed", "file", f.Name, "error", err)
			http.Error(w, "Chunk unavailable; re-upload the file", http.StatusBadGateway)
			return
		}
		if wantExtend {
			s.extendDownload(r.Context(), f.ID)
		}
		if r.Method != http.MethodHead {
			s.trackAsync(f.ID, store.EventDownload, f.Size, r)
		}
		http.Redirect(w, r, cdnURL, http.StatusFound)
		return
	}

	w.Header().Set("Accept-Ranges", "bytes")
	ct, disposition := safeDownloadHeaders(f.Name, isDownload)
	w.Header().Set("Content-Type", ct)
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
	if wantExtend {
		s.extendDownload(r.Context(), f.ID)
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

func (s *Server) extendDownload(ctx context.Context, fileID string) {
	if _, err := s.store.ExtendExpiration(ctx, fileID, s.now().UTC(), downloadExtension, maxRetentionFromNow); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.log.Error("extend expiration failed", "file", fileID, "error", err)
	}
}

// resolveCDNURL returns a signed Discord CDN URL for a part, caching hits.
// When Discord confirms the attachment is gone, the content-addressed chunk
// row is dropped so the next upload re-POSTs those bytes.
func (s *Server) resolveCDNURL(ctx context.Context, part store.FilePart) (string, error) {
	cdnURL, ok := s.cache.GetURL(ctx, part.MessageID)
	if ok {
		return cdnURL, nil
	}
	cdnURL, err := s.discord.AttachmentURL(ctx, part.MessageID, part.BotID)
	if err != nil {
		if errors.Is(err, discord.ErrAttachmentGone) {
			if delErr := s.store.DeleteChunksByMessageID(ctx, part.MessageID); delErr != nil {
				s.log.Error("purge dead chunk failed", "message", part.MessageID, "error", delErr)
			} else {
				s.log.Info("purged dead chunk refs", "message", part.MessageID)
			}
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
	u, ok := s.requireUser(w, r)
	if !ok {
		return
	}
	limit := listLimit
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	files, err := s.store.ListFilesByOwner(r.Context(), u.ID, limit, offset)
	if err != nil {
		s.log.Error("list files failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to list files")
		return
	}
	out := make([]map[string]any, 0, len(files))
	base := s.baseURL(r)
	viewer := &u
	for _, f := range files {
		out = append(out, s.fileLinksResponse(base, f, "", viewer))
	}
	writeJSON(w, http.StatusOK, map[string]any{"files": out})
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	access, err := s.authorizeFileAccess(r, r.PathValue("id"))
	if errors.Is(err, errInvalidID) {
		writeJSONError(w, http.StatusBadRequest, "Invalid file id")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Cannot find the specified file")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if access.ViaToken {
		w.Header().Set("Referrer-Policy", "no-referrer")
	}
	token := ""
	if access.ViaToken {
		token = r.URL.Query().Get("token")
		if token == "" {
			token = r.Header.Get("X-File-Token")
		}
	}
	writeJSON(w, http.StatusOK, s.fileMetaDTO(r, access.File, token, access.User))
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

// newOwnedFile builds a File with ownership, retention, and default visibility
// from the optional session. rawToken is non-empty when the file is private.
func (s *Server) newOwnedFile(r *http.Request, id, name string, size int64, parts []store.FilePart) (store.File, string, error) {
	now := s.now().UTC()
	f := store.File{
		ID:         id,
		Name:       name,
		Size:       size,
		ChunkSize:  chunkSize,
		CreatedAt:  now,
		Visibility: store.VisibilityPublic,
		ExpiresAt:  now.Add(anonymousRetention),
		Parts:      parts,
	}
	if u, ok := s.sessionUser(r); ok {
		uid := u.ID
		f.OwnerUserID = &uid
		f.ExpiresAt = now.Add(authenticatedRetention)
		if u.DefaultVisibility == store.VisibilityPrivate {
			raw, err := auth.GenerateFileToken()
			if err != nil {
				return store.File{}, "", err
			}
			hash := s.keys.HashFileToken(raw)
			rotated := now
			f.Visibility = store.VisibilityPrivate
			f.AccessTokenHash = hash
			f.AccessTokenRotatedAt = &rotated
			return f, raw, nil
		}
	}
	return f, "", nil
}

func (s *Server) writeFileCreated(w http.ResponseWriter, r *http.Request, f store.File, rawToken string) {
	base := s.baseURL(r)
	var viewer *store.User
	if u, ok := s.sessionUser(r); ok {
		viewer = &u
	}
	out := s.fileLinksResponse(base, f, rawToken, viewer)
	if rawToken != "" {
		out["accessToken"] = rawToken
	}
	writeJSON(w, http.StatusOK, out)
}

func ownedByUser(u *store.User, f store.File) bool {
	if u == nil || f.OwnerUserID == nil {
		return false
	}
	return *f.OwnerUserID == u.ID
}

func (s *Server) fileMetaDTO(r *http.Request, f store.File, rawToken string, viewer *store.User) map[string]any {
	base := s.baseURL(r)
	return s.fileLinksResponse(base, f, rawToken, viewer)
}

// fileLinksResponse builds share/download URLs; rawToken is appended for private links.
func (s *Server) fileLinksResponse(base string, f store.File, rawToken string, viewer *store.User) map[string]any {
	urlPath := withToken(base+"/f/"+f.ID, rawToken)
	longPath := withToken(base+"/f/"+f.ID+"/"+f.Name, rawToken)
	dl := withTokenQuery(base+"/f/"+f.ID, "download", "1", rawToken)
	longDL := withTokenQuery(base+"/f/"+f.ID+"/"+f.Name, "download", "1", rawToken)
	return map[string]any{
		"fileId":             f.ID,
		"fileName":           f.Name,
		"fileSize":           f.Size,
		"chunkSize":          f.ChunkSize,
		"visibility":         f.Visibility,
		"ownedByCurrentUser": ownedByUser(viewer, f),
		"createdAt":          f.CreatedAt,
		"expiresAt":          f.ExpiresAt,
		"url":                urlPath,
		"longURL":            longPath,
		"downloadURL":        dl,
		"longDownloadURL":    longDL,
	}
}

func withToken(rawURL, token string) string {
	if token == "" {
		return rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u.String()
}

func withTokenQuery(rawURL, k, v, token string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	q.Set(k, v)
	if token != "" {
		q.Set("token", token)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// fileLinks builds share/download URLs (legacy helper for callers without ownership fields).
func fileLinks(base, fileID, fileName string, fileSize int64) map[string]any {
	return map[string]any{
		"fileId":          fileID,
		"fileName":        fileName,
		"fileSize":        fileSize,
		"url":             base + "/f/" + fileID,
		"longURL":         base + "/f/" + fileID + "/" + fileName,
		"downloadURL":     withTokenQuery(base+"/f/"+fileID, "download", "1", ""),
		"longDownloadURL": withTokenQuery(base+"/f/"+fileID+"/"+fileName, "download", "1", ""),
	}
}

func newID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err) // NewV7 only fails if rand fails
	}
	return hex.EncodeToString(id[:])
}

// parseID accepts dashed or hex UUID forms and returns 32-char lowercase hex.
func parseID(s string) (string, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

var errInvalidID = errors.New("invalid file id")

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"message": message})
}
