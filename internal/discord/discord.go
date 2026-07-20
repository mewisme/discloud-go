// Package discord is a minimal Discord bot API client covering the two
// operations discloud needs: uploading chunk(s) as message attachment(s) and
// re-reading a message to get a fresh signed attachment URL.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	DefaultBaseURL = "https://discord.com/api/v10"
	// MaxAttachments is Discord's per-message file limit.
	MaxAttachments = 10
)

// bot is one Discord bot identity with its own rate-limit clock.
type bot struct {
	token     string
	mu        sync.Mutex
	waitUntil time.Time
}

type Client struct {
	// BaseURL is overridable for tests.
	BaseURL   string
	ChannelID string
	HTTP      *http.Client

	bots []*bot
	next atomic.Uint64 // round-robin index for uploads
}

// Part is one file to attach to a Discord message.
type Part struct {
	Name string
	Data []byte
}

// New builds a client. token may be a single bot token or a comma-separated
// list; uploads are divided round-robin across the tokens.
func New(token, channelID string) *Client {
	tokens := SplitTokens(token)
	bots := make([]*bot, len(tokens))
	for i, t := range tokens {
		bots[i] = &bot{token: t}
	}
	return &Client{
		BaseURL:   DefaultBaseURL,
		ChannelID: channelID,
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
		bots:      bots,
	}
}

// SplitTokens splits a comma-separated token list and drops empty entries.
func SplitTokens(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// TokenCount is how many bots are available for upload distribution.
func (c *Client) TokenCount() int { return len(c.bots) }

// FormatRef builds a stored locator for a Discord attachment.
// Index 0 stays a bare message id for backward compatibility with legacy rows.
func FormatRef(messageID string, idx int) string {
	if idx <= 0 {
		return messageID
	}
	return messageID + ":" + strconv.Itoa(idx)
}

// ParseRef splits a locator into message id and attachment index.
// Bare ids (no ":idx") mean attachment 0.
func ParseRef(ref string) (messageID string, idx int) {
	i := strings.LastIndex(ref, ":")
	if i < 0 {
		return ref, 0
	}
	n, err := strconv.Atoi(ref[i+1:])
	if err != nil || n < 0 {
		return ref, 0
	}
	return ref[:i], n
}

type message struct {
	ID          string `json:"id"`
	Attachments []struct {
		URL string `json:"url"`
	} `json:"attachments"`
}

// UploadChunk posts one attachment and returns a locator (bare message id).
func (c *Client) UploadChunk(ctx context.Context, fileName string, data []byte) (string, error) {
	refs, err := c.UploadParts(ctx, []Part{{Name: fileName, Data: data}})
	if err != nil {
		return "", err
	}
	return refs[0], nil
}

// UploadParts posts up to MaxAttachments files on one message and returns a
// locator per part (messageID or messageID:idx). Successive calls rotate bots.
func (c *Client) UploadParts(ctx context.Context, parts []Part) ([]string, error) {
	if len(c.bots) == 0 {
		return nil, fmt.Errorf("discord: no bot tokens configured")
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("discord: no parts to upload")
	}
	if len(parts) > MaxAttachments {
		return nil, fmt.Errorf("discord: at most %d attachments per message", MaxAttachments)
	}
	for _, p := range parts {
		if len(p.Data) == 0 {
			return nil, fmt.Errorf("discord: empty part %q", p.Name)
		}
	}

	b := c.bots[c.next.Add(1)%uint64(len(c.bots))]
	const maxAttempts = 5
	for attempt := 1; ; attempt++ {
		msg, retryAfter, err := c.postAttachments(ctx, b, parts)
		if err == nil {
			if len(msg.Attachments) < len(parts) {
				return nil, fmt.Errorf("discord: message %s has %d attachments, want %d",
					msg.ID, len(msg.Attachments), len(parts))
			}
			refs := make([]string, len(parts))
			for i := range parts {
				refs[i] = FormatRef(msg.ID, i)
			}
			return refs, nil
		}
		if retryAfter <= 0 || attempt >= maxAttempts {
			return nil, err
		}
		b.backoff(retryAfter)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(b.sleepFor()):
		}
	}
}

// AttachmentURL fetches the message and returns the signed CDN URL for the
// attachment described by ref (bare id or id:idx).
func (c *Client) AttachmentURL(ctx context.Context, ref string) (string, error) {
	if len(c.bots) == 0 {
		return "", fmt.Errorf("discord: no bot tokens configured")
	}
	messageID, idx := ParseRef(ref)
	b := c.bots[0]
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/channels/%s/messages/%s", c.BaseURL, c.ChannelID, messageID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return "", fmt.Errorf("discord: get message %s: %s: %s", messageID, resp.Status, body)
	}
	var msg message
	if err := json.NewDecoder(resp.Body).Decode(&msg); err != nil {
		return "", err
	}
	if idx < 0 || idx >= len(msg.Attachments) {
		return "", fmt.Errorf("discord: message %s has no attachment %d", messageID, idx)
	}
	return msg.Attachments[idx].URL, nil
}

func (c *Client) postAttachments(ctx context.Context, b *bot, parts []Part) (msg message, retryAfter time.Duration, err error) {
	if d := b.sleepFor(); d > 0 {
		select {
		case <-ctx.Done():
			return message{}, 0, ctx.Err()
		case <-time.After(d):
		}
	}

	var total int
	for _, p := range parts {
		total += len(p.Data)
	}
	var body bytes.Buffer
	body.Grow(total + 512*len(parts))
	mw := multipart.NewWriter(&body)
	for i, p := range parts {
		part, err := mw.CreateFormFile(fmt.Sprintf("files[%d]", i), p.Name)
		if err != nil {
			return message{}, 0, err
		}
		if _, err = part.Write(p.Data); err != nil {
			return message{}, 0, err
		}
	}
	if err = mw.Close(); err != nil {
		return message{}, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/channels/%s/messages", c.BaseURL, c.ChannelID), &body)
	if err != nil {
		return message{}, 0, err
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return message{}, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter = parseRetryAfter(resp)
		return message{}, retryAfter, fmt.Errorf("discord: rate limited, retry after %s", retryAfter)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		name := parts[0].Name
		if len(parts) > 1 {
			name = fmt.Sprintf("%s (+%d)", parts[0].Name, len(parts)-1)
		}
		return message{}, 0, fmt.Errorf("discord: upload %s: %s: %s", name, resp.Status, raw)
	}

	if remaining, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining")); err == nil && remaining == 0 {
		if reset, err := strconv.ParseFloat(resp.Header.Get("X-RateLimit-Reset-After"), 64); err == nil {
			b.backoff(time.Duration(reset * float64(time.Second)))
		}
	}

	err = json.NewDecoder(resp.Body).Decode(&msg)
	return msg, 0, err
}

func parseRetryAfter(resp *http.Response) time.Duration {
	var payload struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if json.NewDecoder(resp.Body).Decode(&payload) == nil && payload.RetryAfter > 0 {
		return time.Duration(payload.RetryAfter * float64(time.Second))
	}
	if v, err := strconv.ParseFloat(resp.Header.Get("Retry-After"), 64); err == nil {
		return time.Duration(v * float64(time.Second))
	}
	return time.Second
}

func (b *bot) backoff(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if until := time.Now().Add(d); until.After(b.waitUntil) {
		b.waitUntil = until
	}
}

func (b *bot) sleepFor() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Until(b.waitUntil)
}
