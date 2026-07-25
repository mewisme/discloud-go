// CLI config file load/save: YAML canonical format with JSON migration.
package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	configYAMLName     = "config.yaml"
	configJSONName     = "config.json"
	configJSONMigrated = "config.json.migrated"
)

// SelfUploadConfig holds Discord credentials for self-managed transfers.
type SelfUploadConfig struct {
	BotTokens   []string `yaml:"botTokens,omitempty" json:"botTokens,omitempty"`
	ChannelID   string   `yaml:"channelId,omitempty" json:"channelId,omitempty"`
	GuildID     string   `yaml:"guildId,omitempty" json:"guildId,omitempty"`
	PremiumTier int      `yaml:"premiumTier,omitempty" json:"premiumTier,omitempty"`
	ChunkSize   int64    `yaml:"chunkSize,omitempty" json:"chunkSize,omitempty"` // bytes
}

// Configured reports whether minimum self-upload fields are present.
func (s SelfUploadConfig) Configured() bool {
	return len(NormalizeBotTokens(s.BotTokens)) > 0 &&
		strings.TrimSpace(s.ChannelID) != "" &&
		strings.TrimSpace(s.GuildID) != ""
}

// NormalizeBotTokens trims, drops empty, and deduplicates preserving first order.
func NormalizeBotTokens(tokens []string) []string {
	seen := make(map[string]struct{}, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// FileConfig is the on-disk YAML/JSON schema.
type FileConfig struct {
	Base       string            `yaml:"base,omitempty" json:"base,omitempty"`
	Origin     string            `yaml:"origin,omitempty" json:"origin,omitempty"`
	Token      string            `yaml:"token,omitempty" json:"token,omitempty"`
	SelfUpload *SelfUploadConfig `yaml:"selfUpload,omitempty" json:"selfUpload,omitempty"`
}

// ConfigFilePath is the user config.yaml path (may not exist yet).
func ConfigFilePath() string {
	return filepath.Join(defaultConfigDir(), configYAMLName)
}

// LegacyConfigFilePath is the pre-migration config.json path.
func LegacyConfigFilePath() string {
	return filepath.Join(defaultConfigDir(), configJSONName)
}

// MigratedConfigFilePath is where legacy JSON is archived after migration.
func MigratedConfigFilePath() string {
	return filepath.Join(defaultConfigDir(), configJSONMigrated)
}

// EnsureConfigMigrated migrates config.json → config.yaml when needed.
// Idempotent and failure-safe: leaves JSON untouched on error.
func EnsureConfigMigrated() error {
	yamlPath := ConfigFilePath()
	if _, err := os.Stat(yamlPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	jsonPath := LegacyConfigFilePath()
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var f FileConfig
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("invalid legacy config.json: %w", err)
	}
	normalizeFileConfig(&f)
	if err := writeConfigYAML(yamlPath, f); err != nil {
		return fmt.Errorf("migrate write config.yaml: %w", err)
	}
	// Reopen and validate
	got, err := LoadFileConfig()
	if err != nil {
		_ = os.Remove(yamlPath)
		return fmt.Errorf("migrate verify config.yaml: %w", err)
	}
	if got.Base != f.Base || got.Origin != f.Origin || got.Token != f.Token {
		_ = os.Remove(yamlPath)
		return fmt.Errorf("migrate verify mismatch")
	}
	migrated := MigratedConfigFilePath()
	if err := os.Rename(jsonPath, migrated); err != nil {
		// YAML is good; leave JSON in place rather than lose it
		return fmt.Errorf("migrate archive config.json: %w", err)
	}
	return nil
}

// LoadFileConfig reads config.yaml (running migration first). Missing file → empty.
func LoadFileConfig() (FileConfig, error) {
	if err := EnsureConfigMigrated(); err != nil {
		return FileConfig{}, err
	}
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return FileConfig{}, nil
		}
		return FileConfig{}, err
	}
	var f FileConfig
	if err := yaml.Unmarshal(data, &f); err != nil {
		return FileConfig{}, fmt.Errorf("invalid config.yaml: %w", err)
	}
	normalizeFileConfig(&f)
	return f, nil
}

func loadConfigFile() (base, origin, token string) {
	f, err := LoadFileConfig()
	if err != nil {
		return "", "", ""
	}
	return f.Base, f.Origin, f.Token
}

// LoadSelfUpload returns self-upload config from YAML (may be empty).
func LoadSelfUpload() (SelfUploadConfig, error) {
	f, err := LoadFileConfig()
	if err != nil {
		return SelfUploadConfig{}, err
	}
	if f.SelfUpload == nil {
		return SelfUploadConfig{}, nil
	}
	return *f.SelfUpload, nil
}

// SaveConfigFile merges non-empty base/origin into config.yaml and writes it.
func SaveConfigFile(base, origin string) (path, savedBase, savedOrigin string, err error) {
	path, savedBase, savedOrigin, _, err = SaveConfigFileOpts(base, origin, "", false)
	return path, savedBase, savedOrigin, err
}

// SaveConfigFileOpts is SaveConfigFile plus optional token update.
func SaveConfigFileOpts(base, origin, token string, setToken bool) (path, savedBase, savedOrigin, savedToken string, err error) {
	_ = EnsureConfigMigrated()
	path = ConfigFilePath()
	cur, err := LoadFileConfig()
	if err != nil {
		return "", "", "", "", err
	}
	f := FileConfig{
		Base:       firstNonEmpty(strings.TrimRight(strings.TrimSpace(base), "/"), cur.Base),
		Origin:     firstNonEmpty(strings.TrimRight(strings.TrimSpace(origin), "/"), cur.Origin),
		Token:      cur.Token,
		SelfUpload: cur.SelfUpload,
	}
	if setToken {
		f.Token = strings.TrimSpace(token)
	}
	if f.Base == "" && f.Origin == "" && !setToken {
		return "", "", "", "", fmt.Errorf("nothing to write: pass --base and/or --origin")
	}
	if err := writeConfigYAML(path, f); err != nil {
		return "", "", "", "", err
	}
	return path, f.Base, f.Origin, f.Token, nil
}

// UnsetConfigFile clears the named fields in config.yaml (others kept).
func UnsetConfigFile(clearBase, clearOrigin, clearToken bool) (path string, err error) {
	if !clearBase && !clearOrigin && !clearToken {
		return "", fmt.Errorf("nothing to unset: pass base, origin, and/or token")
	}
	_ = EnsureConfigMigrated()
	path = ConfigFilePath()
	cur, err := LoadFileConfig()
	if err != nil {
		return "", err
	}
	if clearBase {
		cur.Base = ""
	}
	if clearOrigin {
		cur.Origin = ""
	}
	if clearToken {
		cur.Token = ""
	}
	if err := writeConfigYAML(path, cur); err != nil {
		return "", err
	}
	return path, nil
}

// SaveSelfUpload writes or clears the selfUpload section atomically.
func SaveSelfUpload(su *SelfUploadConfig) error {
	_ = EnsureConfigMigrated()
	cur, err := LoadFileConfig()
	if err != nil {
		return err
	}
	if su == nil {
		cur.SelfUpload = nil
	} else {
		cp := *su
		cp.BotTokens = NormalizeBotTokens(cp.BotTokens)
		cp.ChannelID = strings.TrimSpace(cp.ChannelID)
		cp.GuildID = strings.TrimSpace(cp.GuildID)
		cur.SelfUpload = &cp
	}
	return writeConfigYAML(ConfigFilePath(), cur)
}

// UpdateSelfUploadTierAndChunk persists cached tier and chunk size.
func UpdateSelfUploadTierAndChunk(tier int, chunkSize int64) error {
	su, err := LoadSelfUpload()
	if err != nil {
		return err
	}
	if !su.Configured() {
		return fmt.Errorf("self-upload is not configured")
	}
	su.PremiumTier = tier
	su.ChunkSize = chunkSize
	return SaveSelfUpload(&su)
}

func normalizeFileConfig(f *FileConfig) {
	f.Base = strings.TrimRight(strings.TrimSpace(f.Base), "/")
	f.Origin = strings.TrimRight(strings.TrimSpace(f.Origin), "/")
	f.Token = strings.TrimSpace(f.Token)
	if f.SelfUpload != nil {
		f.SelfUpload.BotTokens = NormalizeBotTokens(f.SelfUpload.BotTokens)
		f.SelfUpload.ChannelID = strings.TrimSpace(f.SelfUpload.ChannelID)
		f.SelfUpload.GuildID = strings.TrimSpace(f.SelfUpload.GuildID)
	}
}

func writeConfigYAML(path string, f FileConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	normalizeFileConfig(&f)
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
