package server

import (
	"context"
	"sort"
	"time"

	"github.com/mewisme/discloud-go/internal/store"
)

func (m *memStore) ensureAPITokens() {
	if m.apiTokens == nil {
		m.apiTokens = map[string]store.APIToken{}
	}
}

func (m *memStore) CreateAPIToken(_ context.Context, t store.APIToken) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	m.apiTokens[t.ID] = t
	return nil
}

func (m *memStore) GetAPITokenByHash(_ context.Context, tokenHash string) (store.APIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	for _, t := range m.apiTokens {
		if t.TokenHash == tokenHash {
			return t, nil
		}
	}
	return store.APIToken{}, store.ErrNotFound
}

func (m *memStore) ListAPITokensByUser(_ context.Context, userID string) ([]store.APIToken, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	out := make([]store.APIToken, 0)
	for _, t := range m.apiTokens {
		if t.UserID == userID && t.RevokedAt == nil {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *memStore) RevokeAPIToken(_ context.Context, id, userID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	t, ok := m.apiTokens[id]
	if !ok || t.UserID != userID || t.RevokedAt != nil {
		return store.ErrNotFound
	}
	t.RevokedAt = &now
	m.apiTokens[id] = t
	return nil
}

func (m *memStore) RevokeAPITokensByUser(_ context.Context, userID string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	for id, t := range m.apiTokens {
		if t.UserID == userID && t.RevokedAt == nil {
			t.RevokedAt = &now
			m.apiTokens[id] = t
		}
	}
	return nil
}

func (m *memStore) TouchAPITokenLastUsed(_ context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	t, ok := m.apiTokens[id]
	if !ok {
		return store.ErrNotFound
	}
	t.LastUsedAt = &now
	m.apiTokens[id] = t
	return nil
}

func (m *memStore) CountAPITokensByUser(_ context.Context, userID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureAPITokens()
	var n int64
	for _, t := range m.apiTokens {
		if t.UserID == userID && t.RevokedAt == nil {
			n++
		}
	}
	return n, nil
}
