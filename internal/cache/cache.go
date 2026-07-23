// Package cache caches signed Discord CDN URLs in Valkey so downloads don't
// hit the Discord API for every chunk of every request.
package cache

import (
	"context"
	"net/url"
	"strconv"
	"time"

	"github.com/valkey-io/valkey-go"
)

const defaultTTL = 10 * time.Minute

type Cache struct {
	client valkey.Client
}

// New connects to Valkey. Accepts "host:port" or a valkey://, redis:// URL.
func New(addr string) (*Cache, error) {
	opt, err := valkey.ParseURL(addr)
	if err != nil {
		opt = valkey.ClientOption{InitAddress: []string{addr}}
	}
	client, err := valkey.NewClient(opt)
	if err != nil {
		return nil, err
	}
	return &Cache{client: client}, nil
}

func (c *Cache) Close() { c.client.Close() }

func (c *Cache) Ping(ctx context.Context) error {
	return c.client.Do(ctx, c.client.B().Ping().Build()).Error()
}

// Incr increments a key by 1 and returns the new value.
func (c *Cache) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Do(ctx, c.client.B().Incr().Key(key).Build()).AsInt64()
}

// IncrBy increments a key by n and returns the new value.
func (c *Cache) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return c.client.Do(ctx, c.client.B().Incrby().Key(key).Increment(n).Build()).AsInt64()
}

// Expire sets a TTL on key.
func (c *Cache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	sec := int64(ttl.Seconds())
	if sec < 1 {
		sec = 1
	}
	return c.client.Do(ctx, c.client.B().Expire().Key(key).Seconds(sec).Build()).Error()
}

func (c *Cache) GetURL(ctx context.Context, messageID string) (string, bool) {
	v, err := c.client.Do(ctx, c.client.B().Get().Key(key(messageID)).Build()).ToString()
	return v, err == nil && v != ""
}

// SetURL caches the signed URL until shortly before its embedded expiry.
func (c *Cache) SetURL(ctx context.Context, messageID, cdnURL string) {
	ttl := TTLFromSignedURL(cdnURL, time.Now())
	c.client.Do(ctx, c.client.B().Set().Key(key(messageID)).Value(cdnURL).Ex(ttl).Build())
}

func key(messageID string) string { return "discloud:url:" + messageID }

// TTLFromSignedURL derives a cache TTL from the hex unix timestamp in the
// URL's "ex" query parameter, minus a safety margin. Falls back to a
// conservative default when the URL is not signed as expected.
func TTLFromSignedURL(cdnURL string, now time.Time) time.Duration {
	u, err := url.Parse(cdnURL)
	if err != nil {
		return defaultTTL
	}
	exHex := u.Query().Get("ex")
	if exHex == "" {
		return defaultTTL
	}
	exUnix, err := strconv.ParseInt(exHex, 16, 64)
	if err != nil {
		return defaultTTL
	}
	ttl := time.Unix(exUnix, 0).Sub(now) - time.Minute
	if ttl <= 0 {
		return defaultTTL
	}
	return ttl
}
