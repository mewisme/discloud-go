package client

import "net/http"

// SignUp creates an account and stores the session cookie.
func (c *Client) SignUp(username, password string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/auth/signup", map[string]string{
		"username": username,
		"password": password,
	}, &out)
	return out, err
}

// SignIn authenticates and stores the session cookie.
func (c *Client) SignIn(username, password string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/auth/signin", map[string]string{
		"username": username,
		"password": password,
	}, &out)
	return out, err
}

// SignOut clears the server session and local cookie jar.
func (c *Client) SignOut() error {
	err := c.DoJSON(http.MethodPost, "/api/auth/signout", nil, nil)
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
