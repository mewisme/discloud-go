package client

import "net/http"

// withCookieSession runs fn without Bearer so Set-Cookie is stored (login/signup)
// and Origin is sent for CSRF. Configured PAT is restored afterward and still
// used for later requests when the jar has no session.
func (c *Client) withCookieSession(fn func() error) error {
	tok := c.cfg.Token
	c.cfg.Token = ""
	defer func() { c.cfg.Token = tok }()
	return fn()
}

// SignUp creates an account and stores the session cookie.
func (c *Client) SignUp(username, password string) (map[string]any, error) {
	var out map[string]any
	err := c.withCookieSession(func() error {
		return c.DoJSON(http.MethodPost, "/api/auth/signup", map[string]string{
			"username": username,
			"password": password,
		}, &out)
	})
	return out, err
}

// SignIn authenticates and stores the session cookie.
func (c *Client) SignIn(username, password string) (map[string]any, error) {
	var out map[string]any
	err := c.withCookieSession(func() error {
		return c.DoJSON(http.MethodPost, "/api/auth/signin", map[string]string{
			"username": username,
			"password": password,
		}, &out)
	})
	return out, err
}

// SignOut clears the server session and local cookie jar.
func (c *Client) SignOut() error {
	err := c.withCookieSession(func() error {
		return c.DoJSON(http.MethodPost, "/api/auth/signout", nil, nil)
	})
	_ = c.ClearSession()
	return err
}

// Me returns the account dashboard payload.
func (c *Client) Me() (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodGet, "/api/auth/me", nil, &out)
	return out, err
}

// ChangePassword updates the password (204).
func (c *Client) ChangePassword(current, next string) error {
	return c.DoJSON(http.MethodPost, "/api/auth/password", map[string]string{
		"currentPassword": current,
		"newPassword":     next,
	}, nil)
}

// CreateAPIToken creates a PAT. Raw token is returned once in the response.
func (c *Client) CreateAPIToken(name string, scopes []string, expiresAt string) (map[string]any, error) {
	body := map[string]any{
		"name":   name,
		"scopes": scopes,
	}
	if expiresAt != "" {
		body["expiresAt"] = expiresAt
	}
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/auth/tokens", body, &out)
	return out, err
}

// ListAPITokens returns token metadata (no secrets).
func (c *Client) ListAPITokens() (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodGet, "/api/auth/tokens", nil, &out)
	return out, err
}

// RevokeAPIToken revokes a PAT by id (204).
func (c *Client) RevokeAPIToken(id string) error {
	return c.DoJSON(http.MethodDelete, "/api/auth/tokens/"+id, nil, nil)
}
