package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// ListFiles returns owned files (auth required).
func (c *Client) ListFiles(limit, offset int) (map[string]any, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/api/files"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out map[string]any
	err := c.DoJSON(http.MethodGet, path, nil, &out)
	return out, err
}

// GetFile returns file metadata. token is optional for private files.
func (c *Client) GetFile(id, token string) (map[string]any, error) {
	path := "/api/files/" + url.PathEscape(id)
	if token != "" {
		path += "?token=" + url.QueryEscape(token)
	}
	var out map[string]any
	err := c.DoJSON(http.MethodGet, path, nil, &out)
	return out, err
}

// Inspect returns file analytics.
func (c *Client) Inspect(id, token string) (map[string]any, error) {
	path := "/api/files/" + url.PathEscape(id) + "/inspect"
	if token != "" {
		path += "?token=" + url.QueryEscape(token)
	}
	var out map[string]any
	err := c.DoJSON(http.MethodGet, path, nil, &out)
	return out, err
}

// SetVisibility sets public or private. Private responses include accessToken.
func (c *Client) SetVisibility(id, visibility string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPatch, "/api/files/"+url.PathEscape(id)+"/visibility",
		map[string]string{"visibility": visibility}, &out)
	return out, err
}

// RotateToken issues a new private-file access token.
func (c *Client) RotateToken(id string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/files/"+url.PathEscape(id)+"/access-token/rotate", nil, &out)
	return out, err
}

// DeleteFile removes file metadata (204).
func (c *Client) DeleteFile(id string) error {
	return c.DoJSON(http.MethodDelete, "/api/files/"+url.PathEscape(id), nil, nil)
}

// FileShareUpdate is a PATCH /api/files/{id}/share body.
type FileShareUpdate struct {
	Password     *string `json:"password,omitempty"`
	ExpiresAt    *string `json:"expiresAt,omitempty"`
	MaxDownloads *int    `json:"maxDownloads,omitempty"`
	ShareMode    *string `json:"shareMode,omitempty"`
}

// UpdateShare patches share settings (password, expiry, max downloads, mode).
func (c *Client) UpdateShare(id string, patch FileShareUpdate) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPatch, "/api/files/"+url.PathEscape(id)+"/share", patch, &out)
	return out, err
}

// UnlockFile verifies a share password and sets the unlock cookie in the jar.
func (c *Client) UnlockFile(id, password, token string) error {
	path := "/api/files/" + url.PathEscape(id) + "/unlock"
	b, err := json.Marshal(map[string]string{"password": password})
	if err != nil {
		return err
	}
	headers := map[string]string{}
	if token != "" {
		headers["X-File-Token"] = token
	}
	res, err := c.doWithHeaders(http.MethodPost, path, bytes.NewReader(b), "application/json", headers)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return decodeResponse(res, nil)
}

// RevokeFile force-expires a file and clears its password.
func (c *Client) RevokeFile(id string) error {
	return c.DoJSON(http.MethodPost, "/api/files/"+url.PathEscape(id)+"/revoke", map[string]any{}, nil)
}
