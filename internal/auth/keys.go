// Package auth provides password hashing and domain-separated HMAC helpers.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

const MinPasswordLen = 8

// Keys holds domain-separated HMAC keys derived from APP_SECRET.
type Keys struct {
	Session []byte
	File    []byte
	Upload  []byte
	CSRF    []byte // reserved; Origin check is the primary CSRF defense
}

// DeriveKeys builds HMAC keys from the root app secret.
func DeriveKeys(appSecret string) Keys {
	return Keys{
		Session: hmacSHA256([]byte(appSecret), []byte("discloud/session-token/v1")),
		File:    hmacSHA256([]byte(appSecret), []byte("discloud/file-token/v1")),
		Upload:  hmacSHA256([]byte(appSecret), []byte("discloud/upload-token/v1")),
		CSRF:    hmacSHA256([]byte(appSecret), []byte("discloud/csrf/v1")),
	}
}

func hmacSHA256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

// HashSessionToken returns a hex-encoded HMAC of the raw session token.
func (k Keys) HashSessionToken(raw string) string {
	return hex.EncodeToString(hmacSHA256(k.Session, []byte(raw)))
}

// HashFileToken returns a hex-encoded HMAC of the raw file access token.
func (k Keys) HashFileToken(raw string) string {
	return hex.EncodeToString(hmacSHA256(k.File, []byte(raw)))
}

// FileTokenMatch reports whether raw hashes to wantHash (constant-time).
func (k Keys) FileTokenMatch(raw, wantHash string) bool {
	got := k.HashFileToken(raw)
	return subtle.ConstantTimeCompare([]byte(got), []byte(wantHash)) == 1
}

// HashUploadToken returns a hex-encoded HMAC of the raw upload resume token.
func (k Keys) HashUploadToken(raw string) string {
	return hex.EncodeToString(hmacSHA256(k.Upload, []byte(raw)))
}

// UploadTokenMatch reports whether raw hashes to wantHash (constant-time).
func (k Keys) UploadTokenMatch(raw, wantHash string) bool {
	got := k.HashUploadToken(raw)
	return subtle.ConstantTimeCompare([]byte(got), []byte(wantHash)) == 1
}

// SignFileUnlock returns a hex HMAC payload "fileID|unixExpiry" for unlock cookies.
func (k Keys) SignFileUnlock(fileID string, expiresUnix int64) string {
	msg := fmt.Sprintf("%s|%d", fileID, expiresUnix)
	return hex.EncodeToString(hmacSHA256(k.File, []byte("unlock\x00"+msg))) + "." + fmt.Sprintf("%d", expiresUnix)
}

// FileUnlockMatch reports whether cookieVal is a valid unlock signature for fileID at now.
func (k Keys) FileUnlockMatch(fileID, cookieVal string, nowUnix int64) bool {
	dot := strings.LastIndexByte(cookieVal, '.')
	if dot <= 0 || dot == len(cookieVal)-1 {
		return false
	}
	sig, expStr := cookieVal[:dot], cookieVal[dot+1:]
	var exp int64
	if _, err := fmt.Sscanf(expStr, "%d", &exp); err != nil || exp < nowUnix {
		return false
	}
	want := hex.EncodeToString(hmacSHA256(k.File, []byte(fmt.Sprintf("unlock\x00%s|%d", fileID, exp))))
	return subtle.ConstantTimeCompare([]byte(sig), []byte(want)) == 1
}

// GenerateFileToken returns 32 random bytes as raw URL-safe base64 (no padding).
func GenerateFileToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate file token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// GenerateOpaqueToken returns 32 random bytes as hex (session cookies, ids).
func GenerateOpaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// NormalizeUsername lowercases and trims a username.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// ValidUsername reports whether username matches signup rules (already normalized).
func ValidUsername(username string) bool {
	n := len(username)
	return n >= 3 && n <= 32 && usernamePattern.MatchString(username)
}
