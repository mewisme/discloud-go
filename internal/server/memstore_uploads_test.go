package server

import (
	"context"
	"strings"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (m *memStore) ensureUploads() {
	if m.uploads == nil {
		m.uploads = map[string]store.UploadSession{}
	}
}

func (m *memStore) CreateUploadSession(_ context.Context, u store.UploadSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	if u.Parts == nil {
		u.Parts = map[int]string{}
	}
	m.uploads[u.ID] = u
	return nil
}

func (m *memStore) GetUploadSession(_ context.Context, id string) (store.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[id]
	if !ok {
		return store.UploadSession{}, store.ErrNotFound
	}
	cp := u
	cp.Parts = map[int]string{}
	for k, v := range u.Parts {
		cp.Parts[k] = v
	}
	return cp, nil
}

func (m *memStore) CountOpenUploadSessionsByOwner(_ context.Context, ownerUserID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	var n int64
	for _, u := range m.uploads {
		if u.OwnerUserID != nil && *u.OwnerUserID == ownerUserID &&
			(u.Status == store.UploadPending || u.Status == store.UploadUploading) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) CountOpenUploadSessionsAnon(_ context.Context, fingerprintPrefix string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	var n int64
	for _, u := range m.uploads {
		if u.OwnerUserID == nil &&
			(u.Status == store.UploadPending || u.Status == store.UploadUploading) &&
			strings.HasPrefix(u.ClientFingerprint, fingerprintPrefix) {
			n++
		}
	}
	return n, nil
}

func (m *memStore) RegisterUploadPart(_ context.Context, uploadID string, idx int, hash string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[uploadID]
	if !ok {
		return store.ErrNotFound
	}
	if u.Status != store.UploadPending && u.Status != store.UploadUploading {
		return store.ErrUploadNotActive
	}
	if idx < 0 || idx >= u.ChunkCount {
		return store.ErrNotFound
	}
	if u.Parts == nil {
		u.Parts = map[int]string{}
	}
	if existing, ok := u.Parts[idx]; ok && existing != "" {
		if existing != hash {
			return store.ErrPartConflict
		}
	} else {
		u.Parts[idx] = hash
	}
	u.Status = store.UploadUploading
	u.UpdatedAt = now
	m.uploads[uploadID] = u
	return nil
}

func (m *memStore) BeginUploadComplete(_ context.Context, uploadID string, now time.Time) (store.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[uploadID]
	if !ok {
		return store.UploadSession{}, store.ErrNotFound
	}
	if u.Status == store.UploadCompleted && u.FileID != nil {
		cp := u
		cp.Parts = map[int]string{}
		for k, v := range u.Parts {
			cp.Parts[k] = v
		}
		return cp, nil
	}
	if u.Status != store.UploadPending && u.Status != store.UploadUploading {
		return store.UploadSession{}, store.ErrUploadNotActive
	}
	u.Status = store.UploadCompleting
	u.UpdatedAt = now
	m.uploads[uploadID] = u
	cp := u
	cp.Parts = map[int]string{}
	for k, v := range u.Parts {
		cp.Parts[k] = v
	}
	return cp, nil
}

func (m *memStore) FinishUploadComplete(_ context.Context, uploadID, fileID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[uploadID]
	if !ok || u.Status != store.UploadCompleting {
		return store.ErrNotFound
	}
	u.Status = store.UploadCompleted
	u.FileID = &fileID
	u.UpdatedAt = now
	m.uploads[uploadID] = u
	return nil
}

func (m *memStore) AbortUploadComplete(_ context.Context, uploadID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[uploadID]
	if !ok {
		return nil
	}
	if u.Status == store.UploadCompleting {
		u.Status = store.UploadUploading
		u.UpdatedAt = now
		m.uploads[uploadID] = u
	}
	return nil
}

func (m *memStore) CancelUploadSession(_ context.Context, uploadID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	u, ok := m.uploads[uploadID]
	if !ok {
		return store.ErrNotFound
	}
	if u.Status != store.UploadPending && u.Status != store.UploadUploading {
		return store.ErrUploadNotActive
	}
	u.Status = store.UploadCancelled
	u.UpdatedAt = now
	m.uploads[uploadID] = u
	return nil
}

func (m *memStore) ExpireUploadSessions(_ context.Context, now time.Time, limit int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureUploads()
	var n int64
	for id, u := range m.uploads {
		if n >= int64(limit) {
			break
		}
		if !u.ExpiresAt.After(now) && (u.Status == store.UploadPending || u.Status == store.UploadUploading) {
			u.Status = store.UploadExpired
			u.UpdatedAt = now
			m.uploads[id] = u
			n++
		}
	}
	return n, nil
}
