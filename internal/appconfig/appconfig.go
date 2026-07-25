// Package appconfig merges env defaults with optional Postgres config rows
// for allowlisted public runtime settings.
package appconfig

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/integrity"
	"github.com/mewisme/discloud-go/internal/store"
	"github.com/mewisme/discloud-go/internal/transfer"
)

// Allowlisted public keys (Postgres config table / admin API).
const (
	KeyGuildID        = "guild_id"
	KeyChunkSize      = "chunk_size"
	KeyMaxRawUpload   = "max_raw_upload"
	KeyRLUpload       = "rl_upload"
	KeyRLDownload     = "rl_download"
	KeyMaxUserBytes   = "max_user_bytes"
	KeyMaxAnonUploads = "max_anon_uploads"
	KeyMaxAnonBytes   = "max_anon_bytes"
)

// EnvKey maps an allowlisted config key to its DISCLOUD_* env name.
var EnvKey = map[string]string{
	KeyGuildID:        "DISCLOUD_GUILD_ID",
	KeyChunkSize:      "DISCLOUD_CHUNK_SIZE",
	KeyMaxRawUpload:   "DISCLOUD_MAX_RAW_UPLOAD",
	KeyRLUpload:       "DISCLOUD_RL_UPLOAD",
	KeyRLDownload:     "DISCLOUD_RL_DOWNLOAD",
	KeyMaxUserBytes:   "DISCLOUD_MAX_USER_BYTES",
	KeyMaxAnonUploads: "DISCLOUD_MAX_ANON_UPLOADS",
	KeyMaxAnonBytes:   "DISCLOUD_MAX_ANON_BYTES",
}

// Keys is the stable allowlist order for admin UI / GET responses.
var Keys = []string{
	KeyGuildID,
	KeyChunkSize,
	KeyMaxRawUpload,
	KeyRLUpload,
	KeyRLDownload,
	KeyMaxUserBytes,
	KeyMaxAnonUploads,
	KeyMaxAnonBytes,
}

// IsAllowlisted reports whether key is in the public config allowlist.
func IsAllowlisted(key string) bool {
	_, ok := EnvKey[key]
	return ok
}

// Source is where an effective value came from.
type Source string

const (
	SourceEnv Source = "env"
	SourceDB  Source = "db"
)

// Entry is one merged allowlisted setting.
type Entry struct {
	Key       string  `json:"key"`
	Value     string  `json:"value"`
	Source    Source  `json:"source"`
	UpdatedAt *string `json:"updatedAt,omitempty"`
}

// EnvDefaults extracts string defaults for allowlisted keys from a DISCLOUD_* map
// (merged process/dotenv/yaml/defaults). Missing keys become "".
func EnvDefaults(env map[string]string) map[string]string {
	out := make(map[string]string, len(Keys))
	for _, k := range Keys {
		ek := EnvKey[k]
		out[k] = strings.TrimSpace(env[ek])
	}
	return out
}

// Merge overlays DB rows on env defaults. DB row presence wins (even if value is empty).
func Merge(envDefaults map[string]string, dbRows []store.ConfigEntry) (values map[string]string, sources map[string]Source, updated map[string]string) {
	values = make(map[string]string, len(Keys))
	sources = make(map[string]Source, len(Keys))
	updated = make(map[string]string)
	for _, k := range Keys {
		values[k] = envDefaults[k]
		sources[k] = SourceEnv
	}
	for _, row := range dbRows {
		if !IsAllowlisted(row.Key) {
			continue
		}
		values[row.Key] = row.Value
		sources[row.Key] = SourceDB
		if !row.UpdatedAt.IsZero() {
			updated[row.Key] = row.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z")
		}
	}
	return values, sources, updated
}

// Entries builds the admin GET payload from merge results.
func Entries(values map[string]string, sources map[string]Source, updated map[string]string) []Entry {
	out := make([]Entry, 0, len(Keys))
	for _, k := range Keys {
		e := Entry{Key: k, Value: values[k], Source: sources[k]}
		if t, ok := updated[k]; ok {
			e.UpdatedAt = &t
		}
		out = append(out, e)
	}
	return out
}

// ParseInt64 parses a decimal int64; empty → 0.
func ParseInt64(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// ParseInt parses a decimal int; empty → 0.
func ParseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	return n, err
}

// ResolveEffectiveChunk returns upload chunk size from guild + preferred bytes.
// Empty guildID → integrity.DefaultChunkSize (8 MiB). With guild, fetches
// PremiumTier and applies transfer.ResolveChunkSize(interactive=false).
func ResolveEffectiveChunk(ctx context.Context, dc *discord.Client, guildID string, preferred int64) (size int64, tier int, err error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return integrity.DefaultChunkSize, 0, nil
	}
	if dc == nil {
		return 0, 0, fmt.Errorf("discord client is required when guild_id is set")
	}
	tier, err = dc.GuildPremiumTier(ctx, guildID)
	if err != nil {
		return 0, 0, err
	}
	adj := transfer.ResolveChunkSize(preferred, 0, tier, false)
	return adj.EffectiveSize, tier, nil
}
