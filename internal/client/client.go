// Package client is an HTTP client for the DisCloud API.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const sessionCookie = "discloud_session"

// Config holds API client settings.
type Config struct {
	BaseURL    string // API origin, e.g. http://localhost:8080
	Origin     string // WEB_ORIGIN for CSRF, e.g. http://localhost:3000
	CookiePath string // cookie jar file; empty → default path
}

// DefaultConfig resolves settings in order:
// process env (DISCLOUD_BASE / DISCLOUD_ORIGIN) → config.json → localhost defaults.
// CLI flags (--base / --origin) override after this when set by the caller.
func DefaultConfig() Config {
	envBase := os.Getenv("DISCLOUD_BASE")
	envOrigin := os.Getenv("DISCLOUD_ORIGIN")
	fileBase, fileOrigin := loadConfigFile()
	base := firstNonEmpty(envBase, fileBase, "http://localhost:8080")
	origin := firstNonEmpty(envOrigin, fileOrigin, "http://localhost:3000")
	return Config{
		BaseURL:    strings.TrimRight(base, "/"),
		Origin:     strings.TrimRight(origin, "/"),
		CookiePath: defaultCookiePath(),
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func defaultConfigDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(base, "discloud")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "discloud")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "discloud")
}

func defaultCookiePath() string {
	return filepath.Join(defaultConfigDir(), "cookies")
}

// ConfigFilePath is the user config.json path (may not exist yet).
func ConfigFilePath() string {
	return filepath.Join(defaultConfigDir(), "config.json")
}

type configFile struct {
	Base   string `json:"base"`
	Origin string `json:"origin"`
}

func loadConfigFile() (base, origin string) {
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		return "", ""
	}
	var f configFile
	if json.Unmarshal(data, &f) != nil {
		return "", ""
	}
	return f.Base, f.Origin
}

// SaveConfigFile merges non-empty base/origin into config.json and writes it.
// Empty fields keep the previous file values (or stay empty).
// Returns the path and the values that were written.
func SaveConfigFile(base, origin string) (path, savedBase, savedOrigin string, err error) {
	path = ConfigFilePath()
	curBase, curOrigin := loadConfigFile()
	f := configFile{
		Base:   firstNonEmpty(strings.TrimRight(strings.TrimSpace(base), "/"), curBase),
		Origin: firstNonEmpty(strings.TrimRight(strings.TrimSpace(origin), "/"), curOrigin),
	}
	if f.Base == "" && f.Origin == "" {
		return "", "", "", fmt.Errorf("nothing to write: pass --base and/or --origin")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", "", "", err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return "", "", "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", "", "", err
	}
	return path, f.Base, f.Origin, nil
}

// Error is an API error with HTTP status and message body.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s (%d)", e.Message, e.Status)
	}
	return fmt.Sprintf("request failed (%d)", e.Status)
}

// Client talks to a DisCloud API instance.
type Client struct {
	cfg  Config
	http *http.Client
	jar  *cookiejar.Jar
	mu   sync.Mutex
	base *url.URL
}

// New builds a client and loads any persisted session cookies.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	if cfg.Origin == "" {
		cfg.Origin = "http://localhost:3000"
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.Origin = strings.TrimRight(cfg.Origin, "/")
	if cfg.CookiePath == "" {
		cfg.CookiePath = defaultCookiePath()
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("base URL: %w", err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &Client{
		cfg:  cfg,
		jar:  jar,
		base: u,
		http: &http.Client{Jar: jar},
	}
	if err := c.loadCookies(); err != nil {
		return nil, err
	}
	return c, nil
}

// Config returns a copy of the client config.
func (c *Client) Config() Config { return c.cfg }

// Do performs an HTTP request against the API.
// Mutating methods with a session cookie send Origin for CSRF.
// path may include a query string (e.g. "/api/upload?fileName=x").
func (c *Client) Do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	ref, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u := c.base.ResolveReference(ref)
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("User-Agent", "discloud-cli")
	mutating := method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	if mutating {
		// Match docs curl: Origin must equal WEB_ORIGIN when present or when a session cookie is sent.
		req.Header.Set("Origin", c.cfg.Origin)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if err := c.saveCookies(); err != nil {
		res.Body.Close()
		return nil, err
	}
	return res, nil
}

// DoJSON sends JSON and decodes a JSON response into dst (nil ok for 204).
func (c *Client) DoJSON(method, path string, in any, dst any) error {
	var body io.Reader
	ct := ""
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
		ct = "application/json"
	}
	res, err := c.Do(method, path, body, ct)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return decodeResponse(res, dst)
}

func decodeResponse(res *http.Response, dst any) error {
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		var ej struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(data, &ej) == nil && ej.Message != "" {
			msg = ej.Message
		}
		return &Error{Status: res.StatusCode, Message: msg}
	}
	if dst == nil || res.StatusCode == http.StatusNoContent || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, dst)
}

type cookieFile struct {
	Cookies []cookieEntry `json:"cookies"`
}

type cookieEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Path  string `json:"path"`
}

func (c *Client) loadCookies() error {
	data, err := os.ReadFile(c.cfg.CookiePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var f cookieFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("cookie file: %w", err)
	}
	var cookies []*http.Cookie
	for _, e := range f.Cookies {
		path := e.Path
		if path == "" {
			path = "/"
		}
		cookies = append(cookies, &http.Cookie{Name: e.Name, Value: e.Value, Path: path})
	}
	c.jar.SetCookies(c.base, cookies)
	return nil
}

func (c *Client) saveCookies() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var f cookieFile
	for _, ck := range c.jar.Cookies(c.base) {
		f.Cookies = append(f.Cookies, cookieEntry{Name: ck.Name, Value: ck.Value, Path: ck.Path})
	}
	if err := os.MkdirAll(filepath.Dir(c.cfg.CookiePath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.cfg.CookiePath, data, 0o600)
}

// ClearSession removes persisted cookies.
func (c *Client) ClearSession() error {
	c.jar.SetCookies(c.base, []*http.Cookie{{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1}})
	_ = os.Remove(c.cfg.CookiePath)
	return c.saveCookies()
}
