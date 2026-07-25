package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGuildPremiumTierTokenFallback(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		auth := r.Header.Get("Authorization")
		if auth == "Bot bad" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if auth == "Bot good" && r.URL.Path == "/guilds/g1" {
			_ = json.NewEncoder(w).Encode(map[string]any{"premium_tier": 2})
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewFromTokens([]string{"bad", "good"}, "ch")
	c.BaseURL = srv.URL
	tier, err := c.GuildPremiumTier(context.Background(), "g1")
	if err != nil {
		t.Fatal(err)
	}
	if tier != 2 {
		t.Fatalf("tier=%d", tier)
	}
	if hits < 2 {
		t.Fatalf("hits=%d", hits)
	}
}

func TestGuildPremiumTierNoAccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	c := NewFromTokens([]string{"a"}, "ch")
	c.BaseURL = srv.URL
	_, err := c.GuildPremiumTier(context.Background(), "g1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/channels/ch" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"ch"}`))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c := NewFromTokens([]string{"tok"}, "ch")
	c.BaseURL = srv.URL
	if err := c.ValidateChannel(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	c := NewFromTokens([]string{"tok"}, "ch")
	c.BaseURL = srv.URL
	if err := c.DeleteMessage(context.Background(), "m1", 0); err != nil {
		t.Fatal(err)
	}
}
