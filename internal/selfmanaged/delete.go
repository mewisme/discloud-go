package selfmanaged

import (
	"context"
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/internal/discord"
)

// DeleteResult summarizes remote message deletion.
type DeleteResult struct {
	Deleted      int
	Missing      int
	Unauthorized int
	Failed       int
}

// DeleteRemoteMessages deletes Discord messages for the given files' chunks.
func DeleteRemoteMessages(ctx context.Context, dc *discord.Client, files []FileMeta) DeleteResult {
	var r DeleteResult
	seen := map[string]struct{}{}
	for _, f := range files {
		for _, ch := range f.Chunks {
			if _, ok := seen[ch.MessageID]; ok {
				continue
			}
			seen[ch.MessageID] = struct{}{}
			err := dc.DeleteMessage(ctx, ch.MessageID, ch.BotID)
			if err == nil {
				r.Deleted++
				continue
			}
			if err == discord.ErrAttachmentGone || (err != nil && containsGone(err)) {
				r.Missing++
				continue
			}
			if isAuthError(err) {
				r.Unauthorized++
				continue
			}
			r.Failed++
		}
	}
	return r
}

func containsGone(err error) bool {
	return err != nil && (err == discord.ErrAttachmentGone ||
		fmt.Errorf("%w", err) != nil && // keep simple
			errorStringContains(err, "attachment gone"))
}

func errorStringContains(err error, sub string) bool {
	return err != nil && len(sub) > 0 && (err.Error() == sub || len(err.Error()) >= len(sub) &&
		(stringIndex(err.Error(), sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return stringIndex(msg, "HTTP 401") >= 0 || stringIndex(msg, "HTTP 403") >= 0
}

// RewriteBundleAfterDelete removes files from meta and re-encodes path.
func RewriteBundleAfterDelete(path string, meta *BundleMeta, removeIDs map[string]struct{}, password string) error {
	var keep []FileMeta
	for _, f := range meta.Files {
		if _, drop := removeIDs[f.ID]; drop {
			continue
		}
		keep = append(keep, f)
	}
	meta.Files = keep
	meta.RecalcTotals()
	return Encode(path, meta, EncodeOptions{Password: password})
}

// RemoveLocalBundle deletes the data file unless keep is true.
func RemoveLocalBundle(path string, keep bool) error {
	if keep {
		return nil
	}
	return os.Remove(path)
}
