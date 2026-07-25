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

// ProgressBarConfig holds progress-bar UI settings.
type ProgressBarConfig struct {
	Colors  []string `yaml:"colors,omitempty" json:"colors,omitempty"`
	LongHue bool     `yaml:"longHue,omitempty" json:"longHue,omitempty"` // HSV long-arc blend between stops
}

// TransferUIConfig holds per-direction transfer UI settings.
type TransferUIConfig struct {
	ProgressBar ProgressBarConfig `yaml:"progressbar" json:"progressbar"`
}

// DefaultPromptTheme is the huh theme used when customize.promptTheme is unset.
const DefaultPromptTheme = "base16"

// CustomizeConfig holds CLI UI/UX customization.
type CustomizeConfig struct {
	Upload      *TransferUIConfig `yaml:"upload,omitempty" json:"upload,omitempty"`
	Download    *TransferUIConfig `yaml:"download,omitempty" json:"download,omitempty"`
	PromptTheme string            `yaml:"promptTheme,omitempty" json:"promptTheme,omitempty"` // base16|charm|dracula|catppuccin|default
}

// NormalizePromptTheme validates and normalizes a huh prompt theme id.
// Empty input resolves to DefaultPromptTheme.
func NormalizePromptTheme(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	switch s {
	case "", "base16", "base 16", "base-16":
		return DefaultPromptTheme, nil
	case "charm", "dracula", "catppuccin", "default":
		return s, nil
	default:
		return "", fmt.Errorf("prompt theme: want base16, charm, dracula, catppuccin, or default (got %q)", s)
	}
}

// PromptThemeOrDefault returns the configured prompt theme, or DefaultPromptTheme.
func (c CustomizeConfig) PromptThemeOrDefault() string {
	t, err := NormalizePromptTheme(c.PromptTheme)
	if err != nil {
		return DefaultPromptTheme
	}
	return t
}

// ProgressBarSettings returns colors + longHue for "upload" or "download".
// Empty colors means the caller should use its built-in default palette.
func (c CustomizeConfig) ProgressBarSettings(kind string) (colors []string, longHue bool) {
	var t *TransferUIConfig
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "upload":
		t = c.Upload
	case "download":
		t = c.Download
	}
	if t == nil {
		return nil, false
	}
	out, _ := NormalizeHexColors(t.ProgressBar.Colors)
	return out, t.ProgressBar.LongHue
}

// ProgressColors returns normalized hex colors for "upload" or "download".
// Empty means the caller should use its built-in default palette.
func (c CustomizeConfig) ProgressColors(kind string) []string {
	colors, _ := c.ProgressBarSettings(kind)
	return colors
}

// NormalizeHexColor trims and validates #RGB / #RRGGBB (case-insensitive).
func NormalizeHexColor(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty color")
	}
	if !strings.HasPrefix(s, "#") {
		s = "#" + s
	}
	body := s[1:]
	switch len(body) {
	case 3, 6:
		for _, c := range body {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return "", fmt.Errorf("invalid hex color %q", s)
			}
		}
		return "#" + strings.ToLower(body), nil
	default:
		return "", fmt.Errorf("invalid hex color %q (want #RGB or #RRGGBB)", s)
	}
}

// NormalizeHexColors validates and normalizes a list; drops empties; rejects bad hex.
func NormalizeHexColors(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		c, err := NormalizeHexColor(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// FileConfig is the on-disk YAML/JSON schema.
type FileConfig struct {
	Base       string            `yaml:"base,omitempty" json:"base,omitempty"`
	Origin     string            `yaml:"origin,omitempty" json:"origin,omitempty"`
	Token      string            `yaml:"token,omitempty" json:"token,omitempty"`
	SelfUpload *SelfUploadConfig `yaml:"selfUpload,omitempty" json:"selfUpload,omitempty"`
	Customize  *CustomizeConfig  `yaml:"customize,omitempty" json:"customize,omitempty"`
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
		Customize:  cur.Customize,
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

// LoadCustomize returns UI customize config from YAML (may be empty).
func LoadCustomize() (CustomizeConfig, error) {
	f, err := LoadFileConfig()
	if err != nil {
		return CustomizeConfig{}, err
	}
	if f.Customize == nil {
		return CustomizeConfig{}, nil
	}
	return *f.Customize, nil
}

// SaveCustomize writes or clears the customize section atomically.
func SaveCustomize(c *CustomizeConfig) error {
	_ = EnsureConfigMigrated()
	cur, err := LoadFileConfig()
	if err != nil {
		return err
	}
	if c == nil {
		cur.Customize = nil
		return writeConfigYAML(ConfigFilePath(), cur)
	}
	cp := *c
	if err := validateCustomize(&cp); err != nil {
		return err
	}
	normalizeCustomize(&cp)
	if customizeEmpty(&cp) {
		cur.Customize = nil
	} else {
		cur.Customize = &cp
	}
	return writeConfigYAML(ConfigFilePath(), cur)
}

func customizeEmpty(c *CustomizeConfig) bool {
	return c == nil || (c.Upload == nil && c.Download == nil && strings.TrimSpace(c.PromptTheme) == "")
}

func validateCustomize(c *CustomizeConfig) error {
	if c == nil {
		return nil
	}
	if c.Upload != nil {
		if _, err := NormalizeHexColors(c.Upload.ProgressBar.Colors); err != nil {
			return fmt.Errorf("upload progressbar colors: %w", err)
		}
	}
	if c.Download != nil {
		if _, err := NormalizeHexColors(c.Download.ProgressBar.Colors); err != nil {
			return fmt.Errorf("download progressbar colors: %w", err)
		}
	}
	if strings.TrimSpace(c.PromptTheme) != "" {
		if _, err := NormalizePromptTheme(c.PromptTheme); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCustomize(c *CustomizeConfig) {
	if c == nil {
		return
	}
	c.Upload = normalizeTransferUI(c.Upload)
	c.Download = normalizeTransferUI(c.Download)
	if strings.TrimSpace(c.PromptTheme) == "" {
		c.PromptTheme = ""
		return
	}
	if t, err := NormalizePromptTheme(c.PromptTheme); err == nil {
		c.PromptTheme = t
	}
}

func normalizeTransferUI(t *TransferUIConfig) *TransferUIConfig {
	if t == nil {
		return nil
	}
	colors, err := NormalizeHexColors(t.ProgressBar.Colors)
	if err != nil {
		return nil
	}
	if len(colors) == 0 && !t.ProgressBar.LongHue {
		return nil
	}
	return &TransferUIConfig{ProgressBar: ProgressBarConfig{
		Colors:  colors,
		LongHue: t.ProgressBar.LongHue,
	}}
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
	if f.Customize != nil {
		normalizeCustomize(f.Customize)
		if customizeEmpty(f.Customize) {
			f.Customize = nil
		}
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
