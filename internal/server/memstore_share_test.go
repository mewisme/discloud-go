package server

import (
	"context"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (m *memStore) UpdateFileShare(_ context.Context, id string, p store.FileSharePatch) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.ErrNotFound
	}
	switch {
	case p.ClearPassword:
		f.PasswordHash = ""
	case p.PasswordHash != nil:
		f.PasswordHash = *p.PasswordHash
	}
	if p.ExpiresAt != nil {
		f.ExpiresAt = *p.ExpiresAt
	}
	switch {
	case p.ClearMaxDownloads:
		f.MaxDownloads = nil
	case p.MaxDownloads != nil:
		n := *p.MaxDownloads
		f.MaxDownloads = &n
	}
	if p.ShareMode != nil {
		f.ShareMode = *p.ShareMode
	}
	if f.ShareMode == "" {
		f.ShareMode = store.ShareModeDownload
	}
	m.files[id] = f
	return nil
}

func (m *memStore) IncrementDownloadCount(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.ErrNotFound
	}
	if f.MaxDownloads != nil && f.DownloadCount >= *f.MaxDownloads {
		return store.ErrDownloadLimit
	}
	f.DownloadCount++
	m.files[id] = f
	return nil
}

func (m *memStore) RevokeFile(_ context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.files[id]
	if !ok {
		return store.ErrNotFound
	}
	f.ExpiresAt = now
	f.PasswordHash = ""
	m.files[id] = f
	return nil
}
