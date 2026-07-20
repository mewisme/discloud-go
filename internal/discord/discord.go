// Package discord is a minimal Discord bot API client covering the two
// operations discloud needs: uploading a chunk as a message attachment and
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

const DefaultBaseURL = "https://discord.com/api/v10"

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

type message struct {
	ID          string `json:"id"`
	Attachments []struct {
		URL string `json:"url"`
	} `json:"attachments"`
}

// UploadChunk posts data as a message attachment and returns the message id.
// Successive calls rotate across configured bot tokens.
func (c *Client) UploadChunk(ctx context.Context, fileName string, data []byte) (string, error) {
	if len(c.bots) == 0 {
		return "", fmt.Errorf("discord: no bot tokens configured")
	}
	b := c.bots[c.next.Add(1)%uint64(len(c.bots))]

	const maxAttempts = 5
	for attempt := 1; ; attempt++ {
		msg, retryAfter, err := c.postAttachment(ctx, b, fileName, data)
		if err == nil {
			if len(msg.Attachments) == 0 {
				return "", fmt.Errorf("discord: message %s has no attachments", msg.ID)
			}
			return msg.ID, nil
		}
		if retryAfter <= 0 || attempt >= maxAttempts {
			return "", err
		}
		b.backoff(retryAfter)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(b.sleepFor()):
		}
	}
}

// AttachmentURL fetches the message and returns its first attachment URL,
// which Discord re-signs with a fresh expiry on every read. Any configured
// bot that can see the channel may perform the read.
func (c *Client) AttachmentURL(ctx context.Context, messageID string) (string, error) {
	if len(c.bots) == 0 {
		return "", fmt.Errorf("discord: no bot tokens configured")
	}
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
	if len(msg.Attachments) == 0 {
		return "", fmt.Errorf("discord: message %s has no attachments", messageID)
	}
	return msg.Attachments[0].URL, nil
}

func (c *Client) postAttachment(ctx context.Context, b *bot, fileName string, data []byte) (msg message, retryAfter time.Duration, err error) {
	if d := b.sleepFor(); d > 0 {
		select {
		case <-ctx.Done():
			return message{}, 0, ctx.Err()
		case <-time.After(d):
		}
	}

	var body bytes.Buffer
	body.Grow(len(data) + 512)
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("files[0]", fileName)
	if err != nil {
		return message{}, 0, err
	}
	if _, err = part.Write(data); err != nil {
		return message{}, 0, err
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
		return message{}, 0, fmt.Errorf("discord: upload %s: %s: %s", fileName, resp.Status, raw)
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
