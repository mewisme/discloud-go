// Package config loads server configuration from environment variables.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port             string
	DatabaseURL      string
	ValkeyURL        string
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
		DiscordBotToken:  os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordChannelID: os.Getenv("DISCORD_CHANNEL_ID"),
		PublicBaseURL:    os.Getenv("PUBLIC_BASE_URL"),
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
	return c, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
