// Package transfer provides chunk-size helpers and file transfer utilities
// for self-managed Discord uploads.
package transfer

import (
	"fmt"
)

// MiB is one mebibyte. UI labels this as "MB" per product wording.
const MiB = int64(1 << 20)

// MaxChunkForTier returns the Discord upload attachment limit for a guild
// premium tier. Unknown tiers default to tier 0.
func MaxChunkForTier(tier int) int64 {
	switch tier {
	case 2:
		return 50 * MiB
	case 3:
		return 100 * MiB
	default: // 0, 1, or invalid
		return 10 * MiB
	}
}

// MBToBytes converts integer MB (MiB) to bytes.
func MBToBytes(mb int) int64 {
	if mb <= 0 {
		return 0
	}
	return int64(mb) * MiB
}

// BytesToMB converts bytes to whole MB (MiB), rounding down.
func BytesToMB(bytes int64) int {
	if bytes <= 0 {
		return 0
	}
	return int(bytes / MiB)
}

// FormatChunkMB formats a byte size as "N MB" using whole MiB.
func FormatChunkMB(bytes int64) string {
	return fmt.Sprintf("%d MB", BytesToMB(bytes))
}

// ClampChunkSize returns size if it is within (0, max], else max.
func ClampChunkSize(size, max int64) int64 {
	if max <= 0 {
		return 0
	}
	if size <= 0 || size > max {
		return max
	}
	return size
}

// ValidChunkSize reports whether size is a positive value at or below max.
func ValidChunkSize(size, max int64) bool {
	return size > 0 && max > 0 && size <= max
}

// AdjustResult describes how chunk size should change after a tier check.
type AdjustResult struct {
	Tier          int
	MaxChunk      int64
	EffectiveSize int64
	Downgraded    bool // configured size was above new max; auto-shrunk
	NeedsUpgrade  bool // interactive: offer to increase toward max
	InvalidFixed  bool // configured was zero/negative/missing; replaced
	OldSize       int64
	OldTier       int
}

// ResolveChunkSize applies tier-limit rules.
// interactive=false never increases size; invalid sizes become max.
func ResolveChunkSize(configuredSize int64, oldTier, newTier int, interactive bool) AdjustResult {
	max := MaxChunkForTier(newTier)
	r := AdjustResult{
		Tier:     newTier,
		MaxChunk: max,
		OldSize:  configuredSize,
		OldTier:  oldTier,
	}
	if !ValidChunkSize(configuredSize, max) {
		if configuredSize > max && configuredSize > 0 {
			r.EffectiveSize = max
			r.Downgraded = true
			return r
		}
		// zero, negative, or missing
		r.EffectiveSize = max
		r.InvalidFixed = true
		return r
	}
	r.EffectiveSize = configuredSize
	if interactive && configuredSize < max {
		r.NeedsUpgrade = true
	}
	return r
}
