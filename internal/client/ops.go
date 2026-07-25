package client

import (
	"bytes"
	"context"
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

func (c *Client) GetInfo() (Info, error) {
	var out Info
	err := c.DoJSON(http.MethodGet, "/api/info", nil, &out)
	return out, err
}

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

// AdminOverview is GET /api/admin/overview (admin role / scope).
type AdminOverview struct {
	Storage struct {
		FileCount       int64 `json:"fileCount"`
		TotalBytes      int64 `json:"totalBytes"`
		ChunkStoreCount int64 `json:"chunkStoreCount"`
	} `json:"storage"`
	Users struct {
		Count  int64 `json:"count"`
		Admins int64 `json:"admins"`
	} `json:"users"`
	Uploads struct {
		OpenSessions int64 `json:"openSessions"`
		Completed24h int64 `json:"completed24h"`
		Expired24h   int64 `json:"expired24h"`
		Cancelled24h int64 `json:"cancelled24h"`
	} `json:"uploads"`
	Traffic struct {
		Downloads   int64 `json:"downloads"`
		BytesServed int64 `json:"bytesServed"`
	} `json:"traffic"`
	Bots struct {
		Configured int `json:"configured"`
	} `json:"bots"`
	Deps struct {
		Postgres bool `json:"postgres"`
		Valkey   bool `json:"valkey"`
	} `json:"deps"`
}

// GetAdminOverview fetches instance ops aggregates (requires admin).
func (c *Client) GetAdminOverview() (AdminOverview, error) {
	var out AdminOverview
	err := c.DoJSON(http.MethodGet, "/api/admin/overview", nil, &out)
	return out, err
}

// DownloadOptions controls GET /f/{id}.
type DownloadOptions struct {
	Name     string
	Download bool
	JSON     bool
	Token    string
	Password string
	OutPath  string // empty → return bytes / write stdout by caller
	// Progress reports bytes copied; total is Content-Length when known (else 0).
	Progress func(sent, total int64)
	// Context cancels the body read when cancelled (e.g. Ctrl+C / Esc).
	Context context.Context
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

	var total int64
	if cl := res.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n >= 0 {
			total = n
		}
	}
	ctx := opt.Context
	if ctx == nil {
		ctx = context.Background()
	}
	body := io.Reader(res.Body)
	if opt.Progress != nil || opt.Context != nil {
		if opt.Progress != nil {
			opt.Progress(0, total)
		}
		body = &progressReader{r: res.Body, total: total, report: opt.Progress, ctx: ctx}
	}

	if opt.OutPath != "" {
		f, err := os.Create(opt.OutPath)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		_, err = io.Copy(f, body)
		return nil, err
	}
	return io.ReadAll(body)
}

type progressReader struct {
	r      io.Reader
	total  int64
	sent   int64
	report func(sent, total int64)
	ctx    context.Context
}

func (p *progressReader) Read(b []byte) (int, error) {
	if p.ctx != nil {
		if err := p.ctx.Err(); err != nil {
			return 0, err
		}
	}
	n, err := p.r.Read(b)
	if n > 0 {
		p.sent += int64(n)
		if p.report != nil {
			p.report(p.sent, p.total)
		}
	}
	return n, err
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
