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
	"sync"
	"time"
)

const DefaultBaseURL = "https://discord.com/api/v10"

type Client struct {
	// BaseURL is overridable for tests.
	BaseURL   string
	Token     string
	ChannelID string
	HTTP      *http.Client

	mu        sync.Mutex
	waitUntil time.Time // rate-limit backoff shared across goroutines
}

func New(token, channelID string) *Client {
	return &Client{
		BaseURL:   DefaultBaseURL,
		Token:     token,
		ChannelID: channelID,
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
	}
}

type message struct {
	ID          string `json:"id"`
	Attachments []struct {
		URL string `json:"url"`
	} `json:"attachments"`
}

// UploadChunk posts data as a message attachment and returns the message id.
func (c *Client) UploadChunk(ctx context.Context, fileName string, data []byte) (string, error) {
	const maxAttempts = 5
	for attempt := 1; ; attempt++ {
		msg, retryAfter, err := c.postAttachment(ctx, fileName, data)
		if err == nil {
			if len(msg.Attachments) == 0 {
				return "", fmt.Errorf("discord: message %s has no attachments", msg.ID)
			}
			return msg.ID, nil
		}
		if retryAfter <= 0 || attempt >= maxAttempts {
			return "", err
		}
		c.backoff(retryAfter)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.sleepFor()):
		}
	}
}

// AttachmentURL fetches the message and returns its first attachment URL,
// which Discord re-signs with a fresh expiry on every read.
func (c *Client) AttachmentURL(ctx context.Context, messageID string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/channels/%s/messages/%s", c.BaseURL, c.ChannelID, messageID), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+c.Token)
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

func (c *Client) postAttachment(ctx context.Context, fileName string, data []byte) (msg message, retryAfter time.Duration, err error) {
	// Respect any backoff a concurrent upload already learned about.
	if d := c.sleepFor(); d > 0 {
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
	req.Header.Set("Authorization", "Bot "+c.Token)
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
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return message{}, 0, fmt.Errorf("discord: upload %s: %s: %s", fileName, resp.Status, b)
	}

	// Proactively pause before the bucket empties.
	if remaining, err := strconv.Atoi(resp.Header.Get("X-RateLimit-Remaining")); err == nil && remaining == 0 {
		if reset, err := strconv.ParseFloat(resp.Header.Get("X-RateLimit-Reset-After"), 64); err == nil {
			c.backoff(time.Duration(reset * float64(time.Second)))
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

func (c *Client) backoff(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if until := time.Now().Add(d); until.After(c.waitUntil) {
		c.waitUntil = until
	}
}

func (c *Client) sleepFor() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Until(c.waitUntil)
}
