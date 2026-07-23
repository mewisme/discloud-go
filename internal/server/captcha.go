package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *Server) captchaEnabled() bool {
	return s.captchaSecret != ""
}

// requireCaptcha verifies a Turnstile token when CAPTCHA_SECRET is set (anon only).
func (s *Server) requireCaptcha(w http.ResponseWriter, r *http.Request, token string) bool {
	if !s.captchaEnabled() {
		return true
	}
	if _, ok := s.sessionUser(r); ok {
		return true // signed-in skips captcha
	}
	token = strings.TrimSpace(token)
	if token == "" {
		writeJSONError(w, http.StatusBadRequest, "captchaToken is required")
		return false
	}
	verify := s.captchaVerify
	if verify == nil {
		verify = verifyTurnstileHTTP
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	if err := verify(ctx, s.captchaSecret, token, s.clientIP(r)); err != nil {
		s.log.Info("captcha failed", "error", err)
		writeJSONError(w, http.StatusForbidden, "CAPTCHA verification failed")
		return false
	}
	return true
}

func verifyTurnstileHTTP(ctx context.Context, secret, token, ip string) error {
	form := url.Values{}
	form.Set("secret", secret)
	form.Set("response", token)
	if ip != "" {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://challenges.cloudflare.com/turnstile/v0/siteverify",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return err
	}
	if !out.Success {
		return fmt.Errorf("captcha rejected")
	}
	return nil
}
