// Package config loads server configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port        string
	DatabaseURL string
	ValkeyURL   string
	// DiscordBotToken is the raw env value: one token, or comma-separated
	// tokens that divide uploads across bots.
	DiscordBotToken  string
	DiscordChannelID string
	// PublicBaseURL is used to build share links. When empty, links are
	// derived from the incoming request's Host and forwarded proto.
	PublicBaseURL string
}

func Load() (Config, error) {
	c := Config{
		Port:             getenv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		ValkeyURL:        os.Getenv("VALKEY_URL"),
		DiscordBotToken:  strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordChannelID: os.Getenv("DISCORD_CHANNEL_ID"),
		PublicBaseURL:    strings.TrimSpace(os.Getenv("PUBLIC_BASE_URL")),
	}
	for name, v := range map[string]string{
		"DATABASE_URL":       c.DatabaseURL,
		"VALKEY_URL":         c.ValkeyURL,
		"DISCORD_BOT_TOKEN":  c.DiscordBotToken,
		"DISCORD_CHANNEL_ID": c.DiscordChannelID,
	} {
		if v == "" {
			return Config{}, fmt.Errorf("missing required environment variable %s", name)
		}
	}
	if !hasToken(c.DiscordBotToken) {
		return Config{}, fmt.Errorf("DISCORD_BOT_TOKEN has no usable tokens")
	}
	return c, nil
}

func hasToken(s string) bool {
	for _, p := range strings.Split(s, ",") {
		if strings.TrimSpace(p) != "" {
			return true
		}
	}
	return false
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
