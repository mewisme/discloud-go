package cache

import (
	"fmt"
	"testing"
	"time"
)

func TestTTLFromSignedURL(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)

	// Signed URL expiring in 2h -> TTL is 2h minus the 1m safety margin.
	ex := fmt.Sprintf("%x", now.Add(2*time.Hour).Unix())
	got := TTLFromSignedURL("https://cdn.discordapp.com/attachments/1/2/f?ex="+ex+"&is=abc&hm=def", now)
	if want := 2*time.Hour - time.Minute; got != want {
		t.Errorf("ttl = %v, want %v", got, want)
	}

	for name, u := range map[string]string{
		"no ex param":  "https://cdn.discordapp.com/attachments/1/2/f",
		"bad hex":      "https://cdn.discordapp.com/attachments/1/2/f?ex=zzz",
		"already past": "https://cdn.discordapp.com/attachments/1/2/f?ex=1",
		"unparsable":   "://not a url",
	} {
		if got := TTLFromSignedURL(u, now); got != defaultTTL {
			t.Errorf("%s: ttl = %v, want default %v", name, got, defaultTTL)
		}
	}
}
