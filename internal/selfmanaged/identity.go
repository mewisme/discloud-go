package selfmanaged

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strings"
)

const identityVersion byte = 1

// ComputeConfigIdentity returns a hex-encoded SHA-256 of the canonical
// self-upload destination (guild, channel, sorted unique token hashes).
func ComputeConfigIdentity(guildID, channelID string, botTokens []string) string {
	sum := sha256.Sum256(canonicalIdentityBytes(guildID, channelID, botTokens))
	return hex.EncodeToString(sum[:])
}

// IdentitiesMatch compares two hex identity strings in constant time when possible.
func IdentitiesMatch(a, b string) bool {
	ab, err1 := hex.DecodeString(strings.TrimSpace(a))
	bb, err2 := hex.DecodeString(strings.TrimSpace(b))
	if err1 != nil || err2 != nil || len(ab) != len(bb) {
		return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}

func canonicalIdentityBytes(guildID, channelID string, botTokens []string) []byte {
	guildID = strings.TrimSpace(guildID)
	channelID = strings.TrimSpace(channelID)
	hashes := make([]string, 0, len(botTokens))
	seen := map[string]struct{}{}
	for _, t := range botTokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		h := sha256.Sum256([]byte(t))
		hs := hex.EncodeToString(h[:])
		if _, ok := seen[hs]; ok {
			continue
		}
		seen[hs] = struct{}{}
		hashes = append(hashes, hs)
	}
	sort.Strings(hashes)

	var out []byte
	out = append(out, identityVersion)
	out = appendLP(out, []byte(guildID))
	out = appendLP(out, []byte(channelID))
	for _, h := range hashes {
		raw, _ := hex.DecodeString(h)
		out = appendLP(out, raw)
	}
	return out
}

func appendLP(dst, field []byte) []byte {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(field)))
	dst = append(dst, lenBuf[:]...)
	return append(dst, field...)
}
