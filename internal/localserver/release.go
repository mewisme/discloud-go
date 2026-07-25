package localserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	// DefaultRepo is the GitHub repository releases are pulled from.
	DefaultRepo = "mewisme/discloud-go"

	defaultReleasesBase = "https://github.com"
	defaultAPIBase      = "https://api.github.com"
	checksumsName       = "checksums.txt"
)

// Source resolves and downloads release assets. The zero value targets the
// public repo with a default HTTP client; tests override the base URLs.
type Source struct {
	HTTP         *http.Client
	Repo         string
	ReleasesBase string // https://github.com
	APIBase      string // https://api.github.com
	Token        string // falls back to GH_TOKEN / GITHUB_TOKEN; never logged
	GOOS         string
	GOARCH       string
}

func (s Source) httpClient() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

func (s Source) repo() string { return orDefault(s.Repo, DefaultRepo) }

func (s Source) releasesBase() string {
	return strings.TrimRight(orDefault(s.ReleasesBase, defaultReleasesBase), "/")
}

func (s Source) apiBase() string {
	return strings.TrimRight(orDefault(s.APIBase, defaultAPIBase), "/")
}

func (s Source) goos() string   { return orDefault(s.GOOS, runtime.GOOS) }
func (s Source) goarch() string { return orDefault(s.GOARCH, runtime.GOARCH) }

func (s Source) token() string {
	return orDefault(strings.TrimSpace(s.Token),
		strings.TrimSpace(os.Getenv("GH_TOKEN")),
		strings.TrimSpace(os.Getenv("GITHUB_TOKEN")))
}

func orDefault(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// AssetName is the release archive for this platform, e.g.
// discloud_0.4.1_windows_amd64.zip.
func (s Source) AssetName(version string) string {
	ext := ".tar.gz"
	if s.goos() == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("discloud_%s_%s_%s%s", strings.TrimPrefix(version, "v"), s.goos(), s.goarch(), ext)
}

// ResolveVersion returns a normalized tag. Empty or "latest" queries GitHub:
// the /releases/latest redirect first, then the JSON API.
func (s Source) ResolveVersion(ctx context.Context, version string) (string, error) {
	if v := strings.TrimSpace(version); v != "" && !strings.EqualFold(v, "latest") {
		return NormalizeVersion(v)
	}
	tag, redirectErr := s.latestByRedirect(ctx)
	if redirectErr == nil {
		return NormalizeVersion(tag)
	}
	tag, apiErr := s.latestByAPI(ctx)
	if apiErr == nil {
		return NormalizeVersion(tag)
	}
	return "", fmt.Errorf("resolve latest release: %v; API fallback: %w", redirectErr, apiErr)
}

// latestByRedirect reads the tag out of the Location header without following it.
func (s Source) latestByRedirect(ctx context.Context) (string, error) {
	client := *s.httpClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	url := s.releasesBase() + "/" + s.repo() + "/releases/latest"
	req, err := s.newRequest(ctx, url, "")
	if err != nil {
		return "", err
	}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer drain(res)

	if err := rateLimitError(res); err != nil {
		return "", err
	}
	loc := res.Header.Get("Location")
	_, tag, ok := strings.Cut(loc, "/releases/tag/")
	if !ok {
		return "", fmt.Errorf("no release tag in redirect from %s (status %d)", url, res.StatusCode)
	}
	return strings.Trim(tag, "/"), nil
}

func (s Source) latestByAPI(ctx context.Context) (string, error) {
	url := s.apiBase() + "/repos/" + s.repo() + "/releases/latest"
	req, err := s.newRequest(ctx, url, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	res, err := s.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer drain(res)

	if err := rateLimitError(res); err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, res.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body); err != nil {
		return "", fmt.Errorf("decode %s: %w", url, err)
	}
	if strings.TrimSpace(body.TagName) == "" {
		return "", fmt.Errorf("empty tag_name from %s", url)
	}
	return body.TagName, nil
}

// Download fetches the platform archive into destDir and verifies its SHA-256
// against checksums.txt from the same release.
func (s Source) Download(ctx context.Context, version, destDir string) (string, error) {
	asset := s.AssetName(version)
	base := s.releasesBase() + "/" + s.repo() + "/releases/download/" + version + "/"

	want, err := s.wantedSum(ctx, base+checksumsName, asset)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(destDir, asset)
	sum, err := s.downloadTo(ctx, base+asset, path)
	if err != nil {
		return "", err
	}
	if sum != want {
		_ = os.Remove(path)
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", asset, sum, want)
	}
	return path, nil
}

func (s Source) wantedSum(ctx context.Context, url, asset string) (string, error) {
	req, err := s.newRequest(ctx, url, "")
	if err != nil {
		return "", err
	}
	res, err := s.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, res.Status)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return findChecksum(string(data), asset)
}

// findChecksum reads "<sha256>  <filename>" lines as produced by sha256sum.
func findChecksum(contents, asset string) (string, error) {
	for _, line := range strings.Split(contents, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") == asset {
			sum := strings.ToLower(fields[0])
			if len(sum) != 64 {
				return "", fmt.Errorf("malformed checksum for %s", asset)
			}
			return sum, nil
		}
	}
	return "", fmt.Errorf("%s not listed in %s", asset, checksumsName)
}

func (s Source) downloadTo(ctx context.Context, url, path string) (string, error) {
	req, err := s.newRequest(ctx, url, "")
	if err != nil {
		return "", err
	}
	res, err := s.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer drain(res)
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, res.Status)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), res.Body)
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s Source) newRequest(ctx context.Context, url, accept string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "discloud-cli")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if t := s.token(); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	return req, nil
}

// rateLimitError turns a GitHub throttle into advice the user can act on.
func rateLimitError(res *http.Response) error {
	if res.StatusCode != http.StatusForbidden && res.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if res.Header.Get("X-RateLimit-Remaining") != "0" && res.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	hint := "retry later, pass an explicit --version, or set GH_TOKEN to raise the limit"
	if reset := res.Header.Get("X-RateLimit-Reset"); reset != "" {
		hint = "resets at unix " + reset + "; " + hint
	}
	return fmt.Errorf("GitHub rate limit reached: %s", hint)
}

func drain(res *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<16))
	_ = res.Body.Close()
}
