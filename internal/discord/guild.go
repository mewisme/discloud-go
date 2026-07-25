package discord

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewFromTokens builds a client from a token slice.
func NewFromTokens(tokens []string, channelID string) *Client {
	clean := make([]string, 0, len(tokens))
	seen := map[string]struct{}{}
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		clean = append(clean, t)
	}
	bots := make([]*bot, len(clean))
	for i, t := range clean {
		bots[i] = &bot{token: t}
	}
	return &Client{
		BaseURL:   DefaultBaseURL,
		ChannelID: channelID,
		HTTP:      &http.Client{Timeout: 2 * time.Minute},
		bots:      bots,
	}
}

// GuildPremiumTier fetches premium_tier for guildID, trying tokens until one works.
func (c *Client) GuildPremiumTier(ctx context.Context, guildID string) (int, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return 0, fmt.Errorf("discord: guild id is required")
	}
	if len(c.bots) == 0 {
		return 0, fmt.Errorf("discord: no bot tokens configured")
	}
	var lastErr error
	for _, b := range c.bots {
		tier, err := c.guildTierWith(ctx, b, guildID)
		if err == nil {
			return tier, nil
		}
		lastErr = err
		// skip unauthorized / forbidden; try next token
		if isAuthError(err) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("discord: no usable bot token for guild")
	}
	return 0, fmt.Errorf("discord: cannot read guild %s: %w", guildID, lastErr)
}

func (c *Client) guildTierWith(ctx context.Context, b *bot, guildID string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/guilds/%s", c.BaseURL, guildID), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return 0, &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("discord: get guild: %s: %s", resp.Status, body)
	}
	var g struct {
		PremiumTier int `json:"premium_tier"`
	}
	if err := json.Unmarshal(body, &g); err != nil {
		return 0, err
	}
	if g.PremiumTier < 0 || g.PremiumTier > 3 {
		return 0, fmt.Errorf("discord: invalid premium_tier %d", g.PremiumTier)
	}
	return g.PremiumTier, nil
}

// ValidateChannel checks the configured channel is readable by at least one bot.
func (c *Client) ValidateChannel(ctx context.Context) error {
	if strings.TrimSpace(c.ChannelID) == "" {
		return fmt.Errorf("discord: channel id is required")
	}
	if len(c.bots) == 0 {
		return fmt.Errorf("discord: no bot tokens configured")
	}
	var lastErr error
	for _, b := range c.bots {
		err := c.getChannelWith(ctx, b)
		if err == nil {
			return nil
		}
		lastErr = err
		if isAuthError(err) {
			continue
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable bot token")
	}
	return fmt.Errorf("discord: channel inaccessible: %w", lastErr)
}

func (c *Client) getChannelWith(ctx context.Context, b *bot) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/channels/%s", c.BaseURL, c.ChannelID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord: get channel: %s: %s", resp.Status, body)
	}
	return nil
}

// DeleteMessage deletes a channel message. botHint >= 0 prefers that token slot.
func (c *Client) DeleteMessage(ctx context.Context, messageID string, botHint int) error {
	if len(c.bots) == 0 {
		return fmt.Errorf("discord: no bot tokens configured")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return fmt.Errorf("discord: message id is required")
	}
	try := func(b *bot) error {
		return c.deleteMessageWith(ctx, b, messageID)
	}
	if botHint >= 0 && botHint < len(c.bots) {
		if err := try(c.bots[botHint]); err == nil {
			return nil
		}
	}
	var lastErr error
	for i, b := range c.bots {
		if i == botHint {
			continue
		}
		if err := try(b); err == nil {
			return nil
		} else {
			lastErr = err
			if isAuthError(err) {
				continue
			}
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("delete failed")
	}
	return lastErr
}

func (c *Client) deleteMessageWith(ctx context.Context, b *bot, messageID string) error {
	if d := b.sleepFor(); d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("%s/channels/%s/messages/%s", c.BaseURL, c.ChannelID, messageID), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bot "+b.token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		ra := parseRetryAfter(resp)
		b.backoff(ra)
		return fmt.Errorf("discord: rate limited, retry after %s", ra)
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("%w: message %s", ErrAttachmentGone, messageID)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return &httpStatusError{status: resp.StatusCode, body: string(body)}
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))
		return fmt.Errorf("discord: delete message: %s: %s", resp.Status, body)
	}
	return nil
}

type httpStatusError struct {
	status int
	body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("discord: HTTP %d", e.status)
}

func isAuthError(err error) bool {
	var he *httpStatusError
	if errors.As(err, &he) {
		return he.status == http.StatusUnauthorized || he.status == http.StatusForbidden
	}
	return false
}
