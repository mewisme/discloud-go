package selfmanaged

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/transfer"
	"golang.org/x/sync/errgroup"
)

// DownloadOptions configures downloading files from a bundle.
type DownloadOptions struct {
	Meta            *BundleMeta
	Files           []FileMeta
	OutPath         string // file or directory
	BotTokens       []string
	ChannelID       string
	GuildID         string
	RequireIdentity bool
	DiscordBaseURL  string
	HTTP            *http.Client
	Workers         int
	Progress        func(fileIdx int, sent, total int64)
}

// DownloadResult summarizes a download.
type DownloadResult struct {
	Paths []string
}

// Download reconstructs selected files from Discord using bundle metadata.
func Download(ctx context.Context, opt DownloadOptions) (*DownloadResult, error) {
	if opt.Meta == nil {
		return nil, fmt.Errorf("missing bundle metadata")
	}
	if opt.RequireIdentity {
		want := ComputeConfigIdentity(opt.GuildID, opt.ChannelID, opt.BotTokens)
		if !IdentitiesMatch(want, opt.Meta.ConfigIdentity) {
			return nil, ErrIdentityMismatch
		}
	}
	dc := discord.NewFromTokens(opt.BotTokens, firstNonEmpty(opt.ChannelID, opt.Meta.ChannelID))
	if opt.DiscordBaseURL != "" {
		dc.BaseURL = opt.DiscordBaseURL
	}
	httpClient := opt.HTTP
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = 4
	}

	outDir := opt.OutPath
	singleFile := len(opt.Files) == 1
	var destIsDir bool
	if st, err := os.Stat(opt.OutPath); err == nil && st.IsDir() {
		destIsDir = true
	} else if stringsHasTrailingSep(opt.OutPath) || (!singleFile) {
		destIsDir = true
		_ = os.MkdirAll(opt.OutPath, 0o755)
	}

	var paths []string
	for fi, f := range opt.Files {
		var dest string
		var err error
		if singleFile && !destIsDir && opt.OutPath != "" && !isDirPath(opt.OutPath) {
			dest = opt.OutPath
		} else {
			base := outDir
			if base == "" {
				base = "."
			}
			dest, err = SafeJoin(base, f.LogicalPath)
			if err != nil {
				// fallback to basename only
				dest = filepath.Join(base, filepath.Base(f.OriginalName))
			}
		}
		if err := downloadOneFile(ctx, dc, httpClient, f, dest, workers, func(sent, total int64) {
			if opt.Progress != nil {
				opt.Progress(fi, sent, total)
			}
		}); err != nil {
			return nil, fmt.Errorf("%s: %w", f.OriginalName, err)
		}
		paths = append(paths, dest)
	}
	return &DownloadResult{Paths: paths}, nil
}

func downloadOneFile(ctx context.Context, dc *discord.Client, httpClient *http.Client, f FileMeta, dest string, workers int, progress func(sent, total int64)) error {
	if len(f.Chunks) == 0 {
		return fmt.Errorf("no chunks")
	}
	tmp := dest + ".part"
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		_ = os.Remove(tmp)
	}()

	// preallocate by writing in order via buffer map
	parts := make([][]byte, len(f.Chunks))
	var sent atomicInt64
	report := func() {
		if progress != nil {
			progress(sent.Load(), f.Size)
		}
	}

	type work struct{ idx int }
	jobs := make(chan work, len(f.Chunks))
	for i := range f.Chunks {
		jobs <- work{idx: i}
	}
	close(jobs)

	g, gctx := errgroup.WithContext(ctx)
	var mu sync.Mutex
	for w := 0; w < workers; w++ {
		g.Go(func() error {
			for {
				select {
				case <-gctx.Done():
					return gctx.Err()
				case j, ok := <-jobs:
					if !ok {
						return nil
					}
					ch := f.Chunks[j.idx]
					url, err := dc.AttachmentURL(gctx, ch.MessageID, ch.BotID)
					if err != nil {
						return err
					}
					req, err := http.NewRequestWithContext(gctx, http.MethodGet, url, nil)
					if err != nil {
						return err
					}
					resp, err := httpClient.Do(req)
					if err != nil {
						return err
					}
					data, err := io.ReadAll(io.LimitReader(resp.Body, ch.Size+1))
					resp.Body.Close()
					if err != nil {
						return err
					}
					if resp.StatusCode != http.StatusOK {
						return fmt.Errorf("cdn %s for chunk %d", resp.Status, ch.Index)
					}
					if int64(len(data)) != ch.Size {
						return fmt.Errorf("chunk %d size mismatch", ch.Index)
					}
					if transfer.HashBytes(data) != ch.SHA256 {
						return fmt.Errorf("chunk %d hash mismatch", ch.Index)
					}
					mu.Lock()
					parts[j.idx] = data
					mu.Unlock()
					sent.Add(int64(len(data)))
					report()
				}
			}
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}

	var total int64
	for _, p := range parts {
		if _, err := out.Write(p); err != nil {
			return err
		}
		total += int64(len(p))
	}
	if total != f.Size {
		return fmt.Errorf("file size mismatch: got %d want %d", total, f.Size)
	}
	if err := out.Close(); err != nil {
		return err
	}
	got, err := transfer.HashFile(tmp)
	if err != nil {
		return err
	}
	if got != f.SHA256 {
		return fmt.Errorf("whole-file digest mismatch")
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	return nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func stringsHasTrailingSep(p string) bool {
	return len(p) > 0 && (p[len(p)-1] == '/' || p[len(p)-1] == '\\')
}

func isDirPath(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
