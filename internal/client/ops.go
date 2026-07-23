package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
)

// Info is public upload config from GET /api/info.
type Info struct {
	ChunkSize int64 `json:"chunkSize"`
	Uploads   *struct {
		Sessions    bool  `json:"sessions"`
		MaxFileSize int64 `json:"maxFileSize"`
	} `json:"uploads"`
}

// GetInfo fetches public upload sizing (chunkSize).
func (c *Client) GetInfo() (Info, error) {
	var out Info
	err := c.DoJSON(http.MethodGet, "/api/info", nil, &out)
	return out, err
}

// Health returns the /healthz body.
func (c *Client) Health() (string, error) {
	res, err := c.Do(http.MethodGet, "/healthz", nil, "")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", &Error{Status: res.StatusCode, Message: string(b)}
	}
	return string(b), nil
}

// Ready checks /readyz.
func (c *Client) Ready() (string, error) {
	res, err := c.Do(http.MethodGet, "/readyz", nil, "")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", &Error{Status: res.StatusCode, Message: string(b)}
	}
	return string(b), nil
}

// DownloadOptions controls GET /f/{id}.
type DownloadOptions struct {
	Name     string
	Download bool
	JSON     bool
	Token    string
	Password string
	OutPath  string // empty → return bytes / write stdout by caller
}

// Download fetches a file (or JSON metadata). If OutPath is set, writes there and returns nil body.
func (c *Client) Download(id string, opt DownloadOptions) ([]byte, error) {
	path := "/f/" + url.PathEscape(id)
	if opt.Name != "" {
		path += "/" + url.PathEscape(opt.Name)
	}
	q := url.Values{}
	if opt.Download {
		q.Set("download", "1")
	}
	if opt.JSON {
		q.Set("json", "1")
	}
	if opt.Token != "" {
		q.Set("token", opt.Token)
	}
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	headers := map[string]string{}
	if opt.Password != "" {
		headers["X-File-Password"] = opt.Password
	}
	res, err := c.doWithHeaders(http.MethodGet, path, nil, "", headers)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return nil, &Error{Status: res.StatusCode, Message: string(b)}
	}
	if opt.OutPath != "" {
		f, err := os.Create(opt.OutPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = io.Copy(f, res.Body)
		return nil, err
	}
	return io.ReadAll(res.Body)
}

// ChunkExists reports whether the server has this SHA-256 chunk (HEAD).
func (c *Client) ChunkExists(hash string) (bool, error) {
	res, err := c.Do(http.MethodHead, "/api/chunks/"+hash, nil, "")
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, &Error{Status: res.StatusCode, Message: "chunk check failed"}
	}
}

// PutChunk uploads raw chunk bytes; returns server hash.
func (c *Client) PutChunk(data []byte) (hash string, existed bool, err error) {
	res, err := c.Do(http.MethodPost, "/api/chunks", bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		return "", false, err
	}
	defer res.Body.Close()
	var out struct {
		Hash    string `json:"hash"`
		Existed bool   `json:"existed"`
	}
	if err := decodeResponse(res, &out); err != nil {
		return "", false, err
	}
	return out.Hash, out.Existed, nil
}

// FormatBytes is a tiny helper for progress lines.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
