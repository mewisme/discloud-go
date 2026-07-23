package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mewisme/discloud-go/internal/auth"
	"github.com/mewisme/discloud-go/internal/store"
)

func (s *Server) handleCreateUpload(w http.ResponseWriter, r *http.Request) {
	if !s.allowUploadAuth(w, r) {
		return
	}
	var req struct {
		FileName          string `json:"fileName"`
		FileSize          int64  `json:"fileSize"`
		ClientFingerprint string `json:"clientFingerprint"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if req.FileSize <= 0 {
		writeJSONError(w, http.StatusBadRequest, "fileSize must be positive")
		return
	}
	maxSize := int64(maxChunksPerFile) * chunkSize
	if req.FileSize > maxSize {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("fileSize exceeds maximum (%d bytes)", maxSize))
		return
	}
	if strings.TrimSpace(req.FileName) == "" {
		writeJSONError(w, http.StatusBadRequest, "Missing fileName")
		return
	}
	name := formatFileName(req.FileName)

	chunkCount := int((req.FileSize + chunkSize - 1) / chunkSize)
	if chunkCount > maxChunksPerFile {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Too many chunks (max %d)", maxChunksPerFile))
		return
	}

	now := s.now().UTC()
	var ownerID *string
	ttl := uploadSessionAnonTTL
	if u, ok := s.sessionUser(r); ok {
		uid := u.ID
		ownerID = &uid
		ttl = uploadSessionAuthTTL
		n, err := s.store.CountOpenUploadSessionsByOwner(r.Context(), uid)
		if err != nil {
			s.log.Error("count upload sessions failed", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if n >= maxOpenUploadSessions {
			writeJSONError(w, http.StatusTooManyRequests, "Too many open upload sessions")
			return
		}
	} else {
		ip := s.clientIP(r)
		fp := "ip:" + ip
		n, err := s.store.CountOpenUploadSessionsAnon(r.Context(), fp)
		if err != nil {
			s.log.Error("count anon upload sessions failed", "error", err)
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		if n >= maxOpenUploadSessions {
			writeJSONError(w, http.StatusTooManyRequests, "Too many open upload sessions")
			return
		}
		if req.ClientFingerprint == "" {
			req.ClientFingerprint = fp
		} else if !strings.HasPrefix(req.ClientFingerprint, fp) {
			req.ClientFingerprint = fp + "|" + req.ClientFingerprint
		}
	}

	rawToken, err := auth.GenerateOpaqueToken()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	sess := store.UploadSession{
		ID:                newID(),
		OwnerUserID:       ownerID,
		ResumeTokenHash:   s.keys.HashUploadToken(rawToken),
		FileName:          name,
		FileSize:          req.FileSize,
		ChunkSize:         chunkSize,
		ChunkCount:        chunkCount,
		Status:            store.UploadPending,
		ClientFingerprint: req.ClientFingerprint,
		CreatedAt:         now,
		UpdatedAt:         now,
		ExpiresAt:         now.Add(ttl),
	}
	if err := s.store.CreateUploadSession(r.Context(), sess); err != nil {
		s.log.Error("create upload session failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create upload session")
		return
	}

	missing := make([]int, chunkCount)
	for i := range missing {
		missing[i] = i
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uploadId":       sess.ID,
		"resumeToken":    rawToken,
		"fileName":       sess.FileName,
		"fileSize":       sess.FileSize,
		"chunkSize":      sess.ChunkSize,
		"chunkCount":     sess.ChunkCount,
		"status":         sess.Status,
		"expiresAt":      sess.ExpiresAt,
		"missingIndices": missing,
	})
}

func (s *Server) handleGetUpload(w http.ResponseWriter, r *http.Request) {
	up, ok := s.loadUploadAuthorized(w, r)
	if !ok {
		return
	}
	s.writeUploadProgress(w, r, up)
}

func (s *Server) handleRegisterUploadPart(w http.ResponseWriter, r *http.Request) {
	up, ok := s.loadUploadAuthorized(w, r)
	if !ok {
		return
	}
	idx, err := strconv.Atoi(r.PathValue("idx"))
	if err != nil || idx < 0 {
		writeJSONError(w, http.StatusBadRequest, "Invalid part index")
		return
	}
	var body struct {
		Hash string `json:"hash"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if !hashPattern.MatchString(body.Hash) {
		writeJSONError(w, http.StatusBadRequest, "Invalid chunk hash")
		return
	}
	if err := s.registerOnePart(w, r, up, idx, body.Hash); err != nil {
		return
	}
	up, err = s.store.GetUploadSession(r.Context(), up.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.writeUploadProgress(w, r, up)
}

func (s *Server) handleRegisterUploadParts(w http.ResponseWriter, r *http.Request) {
	up, ok := s.loadUploadAuthorized(w, r)
	if !ok {
		return
	}
	var body struct {
		Parts []struct {
			Idx  int    `json:"idx"`
			Hash string `json:"hash"`
		} `json:"parts"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}
	if len(body.Parts) == 0 {
		writeJSONError(w, http.StatusBadRequest, "Missing parts")
		return
	}
	for _, p := range body.Parts {
		if !hashPattern.MatchString(p.Hash) {
			writeJSONError(w, http.StatusBadRequest, "Invalid chunk hash")
			return
		}
		if err := s.registerOnePart(w, r, up, p.Idx, p.Hash); err != nil {
			return
		}
	}
	up, err := s.store.GetUploadSession(r.Context(), up.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	s.writeUploadProgress(w, r, up)
}

func (s *Server) registerOnePart(w http.ResponseWriter, r *http.Request, up store.UploadSession, idx int, hash string) error {
	if up.Status == store.UploadCancelled || up.Status == store.UploadExpired {
		writeJSONError(w, http.StatusGone, "Upload session is no longer active")
		return errUploadGone
	}
	if !up.ExpiresAt.After(s.now().UTC()) && (up.Status == store.UploadPending || up.Status == store.UploadUploading) {
		writeJSONError(w, http.StatusGone, "Upload session expired")
		return errUploadGone
	}
	exists, err := s.store.HasChunk(r.Context(), hash)
	if err != nil {
		s.log.Error("chunk lookup failed", "hash", hash, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return err
	}
	if !exists {
		writeJSONError(w, http.StatusBadRequest, "Unknown chunk hash: "+hash)
		return errUploadBad
	}
	err = s.store.RegisterUploadPart(r.Context(), up.ID, idx, hash, s.now().UTC())
	if errors.Is(err, store.ErrPartConflict) {
		writeJSONError(w, http.StatusConflict, "Part hash conflict")
		return err
	}
	if errors.Is(err, store.ErrUploadNotActive) {
		writeJSONError(w, http.StatusGone, "Upload session is no longer active")
		return err
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusBadRequest, "Invalid part index")
		return err
	}
	if err != nil {
		s.log.Error("register part failed", "upload", up.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return err
	}
	return nil
}

var (
	errUploadGone = errors.New("upload gone")
	errUploadBad  = errors.New("upload bad request")
)

func (s *Server) handleCompleteUploadSession(w http.ResponseWriter, r *http.Request) {
	up, ok := s.loadUploadAuthorized(w, r)
	if !ok {
		return
	}
	now := s.now().UTC()
	if up.Status != store.UploadCompleted &&
		(up.Status == store.UploadCancelled || up.Status == store.UploadExpired || !up.ExpiresAt.After(now)) {
		writeJSONError(w, http.StatusGone, "Upload session is no longer active")
		return
	}

	sess, err := s.store.BeginUploadComplete(r.Context(), up.ID, now)
	if errors.Is(err, store.ErrUploadNotActive) {
		writeJSONError(w, http.StatusGone, "Upload session is no longer active")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Upload session not found")
		return
	}
	if err != nil {
		s.log.Error("begin complete failed", "upload", up.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if sess.Status == store.UploadCompleted && sess.FileID != nil {
		f, err := s.store.GetFile(r.Context(), *sess.FileID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Internal server error")
			return
		}
		s.writeFileCreated(w, r, f, "")
		return
	}

	hashes := make([]string, sess.ChunkCount)
	for i := 0; i < sess.ChunkCount; i++ {
		h, ok := sess.Parts[i]
		if !ok || h == "" {
			_ = s.store.AbortUploadComplete(r.Context(), sess.ID, now)
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("Missing part %d", i))
			return
		}
		hashes[i] = h
	}

	f, rawToken, err := s.assembleFileFromHashes(r, sess.FileName, hashes)
	if err != nil {
		_ = s.store.AbortUploadComplete(r.Context(), sess.ID, now)
		var httpErr *uploadHTTPError
		if errors.As(err, &httpErr) {
			writeJSONError(w, httpErr.code, httpErr.msg)
			return
		}
		s.log.Error("assemble failed", "upload", sess.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to assemble file")
		return
	}

	if err := s.store.FinishUploadComplete(r.Context(), sess.ID, f.ID, now); err != nil {
		s.log.Error("finish complete failed", "upload", sess.ID, "error", err)
	}
	s.writeFileCreated(w, r, f, rawToken)
}

type uploadHTTPError struct {
	code int
	msg  string
}

func (e *uploadHTTPError) Error() string { return e.msg }

// assembleFileFromHashes resolves hashes, dedupes for owner, or creates a new file.
func (s *Server) assembleFileFromHashes(r *http.Request, fileName string, hashes []string) (store.File, string, error) {
	known, err := s.store.GetChunks(r.Context(), hashes)
	if err != nil {
		return store.File{}, "", err
	}
	var fileSize int64
	parts := make([]store.FilePart, len(hashes))
	messageIDs := make([]string, len(hashes))
	for i, h := range hashes {
		c, ok := known[h]
		if !ok {
			return store.File{}, "", &uploadHTTPError{http.StatusBadRequest, "Unknown chunk hash: " + h}
		}
		if i < len(hashes)-1 && c.Size != chunkSize {
			return store.File{}, "", &uploadHTTPError{http.StatusBadRequest,
				fmt.Sprintf("Chunk %d is %d bytes; every chunk except the last must be exactly %d bytes", i, c.Size, chunkSize)}
		}
		fileSize += c.Size
		parts[i] = store.FilePart{MessageID: c.MessageID, BotID: c.BotID}
		messageIDs[i] = c.MessageID
	}
	name := formatFileName(fileName)
	if u, ok := s.sessionUser(r); ok {
		uid := u.ID
		existing, err := s.store.FindFileByNameAndParts(r.Context(), &uid, name, messageIDs, s.now().UTC())
		if err == nil {
			if existing.Status != store.FileStatusReused {
				if err := s.store.UpdateFileStatus(r.Context(), existing.ID, store.FileStatusReused); err != nil {
					return store.File{}, "", err
				}
				existing.Status = store.FileStatusReused
			}
			return existing, "", nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return store.File{}, "", err
		}
	}
	f, rawToken, err := s.newOwnedFile(r, newID(), name, fileSize, parts)
	if err != nil {
		return store.File{}, "", err
	}
	if err := s.store.CreateFile(r.Context(), f); err != nil {
		return store.File{}, "", err
	}
	return f, rawToken, nil
}

func (s *Server) handleCancelUpload(w http.ResponseWriter, r *http.Request) {
	up, ok := s.loadUploadAuthorized(w, r)
	if !ok {
		return
	}
	if up.Status == store.UploadCompleted {
		writeJSONError(w, http.StatusConflict, "Upload already completed")
		return
	}
	if up.Status == store.UploadCancelled || up.Status == store.UploadExpired {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	err := s.store.CancelUploadSession(r.Context(), up.ID, s.now().UTC())
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Upload session not found")
		return
	}
	if errors.Is(err, store.ErrUploadNotActive) {
		writeJSONError(w, http.StatusConflict, "Upload session is no longer cancellable")
		return
	}
	if err != nil {
		s.log.Error("cancel upload failed", "upload", up.ID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) loadUploadAuthorized(w http.ResponseWriter, r *http.Request) (store.UploadSession, bool) {
	if !s.allowUploadAuth(w, r) {
		return store.UploadSession{}, false
	}
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid upload id")
		return store.UploadSession{}, false
	}
	up, err := s.store.GetUploadSession(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "Upload session not found")
		return store.UploadSession{}, false
	}
	if err != nil {
		s.log.Error("get upload failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return store.UploadSession{}, false
	}
	if !s.authorizeUploadAccess(r, up) {
		writeJSONError(w, http.StatusNotFound, "Upload session not found")
		return store.UploadSession{}, false
	}
	return up, true
}

func (s *Server) authorizeUploadAccess(r *http.Request, up store.UploadSession) bool {
	if u, ok := s.sessionUser(r); ok {
		if up.OwnerUserID != nil && *up.OwnerUserID == u.ID {
			return true
		}
		if u.Role == store.RoleAdmin {
			return true
		}
	}
	raw := r.Header.Get(uploadTokenHeader)
	if raw == "" {
		raw = r.URL.Query().Get("token")
	}
	return raw != "" && s.keys.UploadTokenMatch(raw, up.ResumeTokenHash)
}

func (s *Server) writeUploadProgress(w http.ResponseWriter, r *http.Request, up store.UploadSession) {
	if up.Parts == nil {
		up.Parts = map[int]string{}
	}
	missing := make([]int, 0)
	registered := make(map[string]any, len(up.Parts))
	var bytesReg int64
	hashes := make([]string, 0, len(up.Parts))
	for i := 0; i < up.ChunkCount; i++ {
		h, ok := up.Parts[i]
		if !ok || h == "" {
			missing = append(missing, i)
			continue
		}
		registered[strconv.Itoa(i)] = h
		hashes = append(hashes, h)
	}
	known, err := s.store.GetChunks(r.Context(), hashes)
	if err != nil {
		s.log.Error("chunk resolve failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	unknown := make([]int, 0)
	for i := 0; i < up.ChunkCount; i++ {
		h, ok := up.Parts[i]
		if !ok || h == "" {
			continue
		}
		c, ok := known[h]
		if !ok {
			unknown = append(unknown, i)
			continue
		}
		bytesReg += c.Size
	}
	status := up.Status
	now := s.now().UTC()
	if (status == store.UploadPending || status == store.UploadUploading) && !up.ExpiresAt.After(now) {
		status = store.UploadExpired
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"uploadId":          up.ID,
		"fileName":          up.FileName,
		"fileSize":          up.FileSize,
		"chunkSize":         up.ChunkSize,
		"chunkCount":        up.ChunkCount,
		"status":            status,
		"expiresAt":         up.ExpiresAt,
		"fileId":            up.FileID,
		"parts":             registered,
		"missingIndices":    missing,
		"unknownIndices":    unknown,
		"bytesRegistered":   bytesReg,
		"clientFingerprint": up.ClientFingerprint,
	})
}
