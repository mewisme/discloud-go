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
	Token      string // personal access token (Bearer); env DISCLOUD_TOKEN / config.json
}

// DefaultConfig resolves settings in order:
// process env (DISCLOUD_BASE / DISCLOUD_ORIGIN / DISCLOUD_TOKEN) → config.json → localhost defaults.
// CLI flags (--base / --origin) override after this when set by the caller.
// Auth priority at request time: session cookie jar > Token (env/config).
func DefaultConfig() Config {
	envBase := os.Getenv("DISCLOUD_BASE")
	envOrigin := os.Getenv("DISCLOUD_ORIGIN")
	envToken := os.Getenv("DISCLOUD_TOKEN")
	fileBase, fileOrigin, fileToken := loadConfigFile()
	base := firstNonEmpty(envBase, fileBase, "http://localhost:8080")
	origin := firstNonEmpty(envOrigin, fileOrigin, "http://localhost:3000")
	token := firstNonEmpty(envToken, fileToken)
	return Config{
		BaseURL:    strings.TrimRight(base, "/"),
		Origin:     strings.TrimRight(origin, "/"),
		CookiePath: defaultCookiePath(),
		Token:      strings.TrimSpace(token),
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
	Token  string `json:"token,omitempty"`
}

func loadConfigFile() (base, origin, token string) {
	data, err := os.ReadFile(ConfigFilePath())
	if err != nil {
		return "", "", ""
	}
	var f configFile
	if json.Unmarshal(data, &f) != nil {
		return "", "", ""
	}
	return f.Base, f.Origin, f.Token
}

// SaveConfigFile merges non-empty base/origin into config.json and writes it.
// Empty fields keep the previous file values (or stay empty). Token is preserved.
// Returns the path and the values that were written.
func SaveConfigFile(base, origin string) (path, savedBase, savedOrigin string, err error) {
	path, savedBase, savedOrigin, _, err = SaveConfigFileOpts(base, origin, "", false)
	return path, savedBase, savedOrigin, err
}

// SaveConfigFileOpts is SaveConfigFile plus optional token update.
// When setToken is true, token is written as-is (empty clears the stored token).
func SaveConfigFileOpts(base, origin, token string, setToken bool) (path, savedBase, savedOrigin, savedToken string, err error) {
	path = ConfigFilePath()
	curBase, curOrigin, curToken := loadConfigFile()
	f := configFile{
		Base:   firstNonEmpty(strings.TrimRight(strings.TrimSpace(base), "/"), curBase),
		Origin: firstNonEmpty(strings.TrimRight(strings.TrimSpace(origin), "/"), curOrigin),
		Token:  curToken,
	}
	if setToken {
		f.Token = strings.TrimSpace(token)
	}
	if f.Base == "" && f.Origin == "" && !setToken {
		return "", "", "", "", fmt.Errorf("nothing to write: pass --base and/or --origin")
	}
	if err := writeConfigFile(path, f); err != nil {
		return "", "", "", "", err
	}
	return path, f.Base, f.Origin, f.Token, nil
}

// UnsetConfigFile clears the named fields in config.json (others kept).
func UnsetConfigFile(clearBase, clearOrigin, clearToken bool) (path string, err error) {
	if !clearBase && !clearOrigin && !clearToken {
		return "", fmt.Errorf("nothing to unset: pass base, origin, and/or token")
	}
	path = ConfigFilePath()
	curBase, curOrigin, curToken := loadConfigFile()
	f := configFile{Base: curBase, Origin: curOrigin, Token: curToken}
	if clearBase {
		f.Base = ""
	}
	if clearOrigin {
		f.Origin = ""
	}
	if clearToken {
		f.Token = ""
	}
	if err := writeConfigFile(path, f); err != nil {
		return "", err
	}
	return path, nil
}

func writeConfigFile(path string, f configFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
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
	// Empty CookiePath = ephemeral jar (no load/save); used by auth token info.
	if cfg.CookiePath != "" {
		if err := c.loadCookies(); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Config returns a copy of the client config.
func (c *Client) Config() Config { return c.cfg }

// hasSessionCookie reports whether the jar has a non-empty discloud_session.
func (c *Client) hasSessionCookie() bool {
	for _, ck := range c.jar.Cookies(c.base) {
		if ck.Name == sessionCookie && strings.TrimSpace(ck.Value) != "" {
			return true
		}
	}
	return false
}

// useBearer is true when a PAT should be sent. Session cookie in the jar wins
// over Token from env/config.json (jar > config).
func (c *Client) useBearer() bool {
	if c.hasSessionCookie() {
		return false
	}
	return strings.TrimSpace(c.cfg.Token) != ""
}

// Do performs an HTTP request against the API.
// Mutating methods with a session cookie send Origin for CSRF.
// When using a PAT (no session cookie), sends Authorization: Bearer and omits Origin.
// path may include a query string (e.g. "/api/upload?fileName=x").
func (c *Client) Do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	return c.doWithHeaders(method, path, body, contentType, nil)
}

// DoJSON sends JSON and decodes a JSON response into dst (nil ok for 204).
func (c *Client) DoJSON(method, path string, in any, dst any) error {
	return c.DoJSONUploadToken(method, path, in, dst, "")
}

// DoJSONUploadToken sends JSON; when token is non-empty sets X-Upload-Token.
func (c *Client) DoJSONUploadToken(method, path string, in any, dst any, token string) error {
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
	headers := map[string]string{}
	if token != "" {
		headers["X-Upload-Token"] = token
	}
	res, err := c.doWithHeaders(method, path, body, ct, headers)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return decodeResponse(res, dst)
}

func (c *Client) doWithHeaders(method, path string, body io.Reader, contentType string, headers map[string]string) (*http.Response, error) {
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
	useBearer := c.useBearer()
	if useBearer {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.cfg.Token))
	}
	mutating := method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
	if mutating && !useBearer {
		req.Header.Set("Origin", c.cfg.Origin)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// Always use the jar client so Set-Cookie (e.g. auth login) is stored even when
	// this request sends Bearer. Outbound Cookie is empty until a session exists;
	// once the jar has discloud_session, useBearer is false and cookie auth wins.
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if c.cfg.CookiePath != "" {
		if err := c.saveCookies(); err != nil {
			res.Body.Close()
			return nil, err
		}
	}
	return res, nil
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
	if c.cfg.CookiePath == "" {
		return nil
	}
	_ = os.Remove(c.cfg.CookiePath)
	return c.saveCookies()
}
