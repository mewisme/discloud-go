package selfmanaged

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/transfer"
	"golang.org/x/sync/errgroup"
)

// UploadJob is one local file to include in a bundle.
type UploadJob struct {
	Path       string
	RemoteName string // logical path / original name
	Size       int64
}

// UploadOptions configures a self-managed upload.
type UploadOptions struct {
	Jobs           []UploadJob
	OutPath        string
	Password       string
	GuildID        string
	ChannelID      string
	BotTokens      []string
	ChunkSize      int64
	PremiumTier    int
	Workers        int    // 0 → auto: one per bot token, capped by chunks / 32
	DiscordBaseURL string // tests
	Progress       func(fileIdx int, sent, total int64)
	OnRetry        func()
}

// UploadResult is returned after a successful self upload.
type UploadResult struct {
	BundleID    string
	DataFile    string
	Files       int
	TotalSize   int64
	PremiumTier int
	ChunkSize   int64
	Protection  string
	Workers     int
}

// maxUploadWorkers caps auto/explicit parallel Discord chunk workers.
const maxUploadWorkers = 32

// ResolveWorkers picks parallel chunk workers for Discord uploads.
// requested <= 0 → auto-scale to one worker per bot token.
// Always capped by chunkCount (when > 0) and maxUploadWorkers.
func ResolveWorkers(requested, tokenCount, chunkCount int) int {
	if tokenCount < 1 {
		tokenCount = 1
	}
	w := requested
	if w <= 0 {
		w = tokenCount
	}
	if chunkCount > 0 && w > chunkCount {
		w = chunkCount
	}
	if w > maxUploadWorkers {
		w = maxUploadWorkers
	}
	if w < 1 {
		w = 1
	}
	return w
}

// Upload chunks files to Discord and writes one .discloud bundle.
func Upload(ctx context.Context, opt UploadOptions) (*UploadResult, error) {
	if len(opt.Jobs) == 0 {
		return nil, fmt.Errorf("no files to upload")
	}
	if opt.ChunkSize <= 0 {
		return nil, fmt.Errorf("invalid chunk size")
	}
	tokens := opt.BotTokens
	if len(tokens) == 0 {
		return nil, fmt.Errorf("no bot tokens")
	}
	dc := discord.NewFromTokens(tokens, opt.ChannelID)
	if opt.DiscordBaseURL != "" {
		dc.BaseURL = opt.DiscordBaseURL
	}
	identity := ComputeConfigIdentity(opt.GuildID, opt.ChannelID, tokens)
	workersHint := opt.Workers

	meta := &BundleMeta{
		BundleID:       uuid.NewString(),
		FormatVersion:  MetadataVersion,
		CreatedAt:      time.Now().UTC(),
		ConfigIdentity: identity,
		GuildID:        opt.GuildID,
		ChannelID:      opt.ChannelID,
		PremiumTier:    opt.PremiumTier,
		ChunkSize:      opt.ChunkSize,
	}

	for fi, job := range opt.Jobs {
		fm, err := uploadOneFile(ctx, dc, job, opt.ChunkSize, workersHint, func(sent, total int64) {
			if opt.Progress != nil {
				opt.Progress(fi, sent, total)
			}
		})
		if err != nil {
			return nil, fmt.Errorf("%s: %w", job.RemoteName, err)
		}
		meta.Files = append(meta.Files, *fm)
	}
	meta.RecalcTotals()

	encOpt := EncodeOptions{Password: opt.Password}
	if err := Encode(opt.OutPath, meta, encOpt); err != nil {
		return nil, err
	}
	prot := ProtectionObfuscated
	if opt.Password != "" {
		prot = ProtectionPassword
	}
	return &UploadResult{
		BundleID:    meta.BundleID,
		DataFile:    opt.OutPath,
		Files:       meta.FileCount,
		TotalSize:   meta.TotalSize,
		PremiumTier: meta.PremiumTier,
		ChunkSize:   meta.ChunkSize,
		Protection:  prot,
		Workers:     ResolveWorkers(workersHint, dc.TokenCount(), 0),
	}, nil
}

func uploadOneFile(ctx context.Context, dc *discord.Client, job UploadJob, chunkSize int64, workersHint int, progress func(sent, total int64)) (*FileMeta, error) {
	digest, err := transfer.HashFile(job.Path)
	if err != nil {
		return nil, err
	}
	nChunks := transfer.ChunkCount(job.Size, chunkSize)
	workers := ResolveWorkers(workersHint, dc.TokenCount(), nChunks)
	chunks := make([]ChunkMeta, nChunks)
	var sent atomicInt64
	report := func() {
		if progress != nil {
			progress(sent.Load(), job.Size)
		}
	}
	report()

	type work struct{ idx int }
	jobs := make(chan work, nChunks)
	for i := 0; i < nChunks; i++ {
		jobs <- work{idx: i}
	}
	close(jobs)

	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
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
					data, err := transfer.ReadChunk(job.Path, j.idx, chunkSize, job.Size)
					if err != nil {
						return err
					}
					hash := transfer.HashBytes(data)
					name := fmt.Sprintf("chunk-%d.bin", j.idx)
					res, err := dc.UploadChunk(gctx, name, data)
					if err != nil {
						return err
					}
					mu.Lock()
					chunks[j.idx] = ChunkMeta{
						Index:          j.idx,
						Size:           int64(len(data)),
						SHA256:         hash,
						MessageID:      res.MessageID,
						AttachmentID:   res.AttachmentID,
						AttachmentName: res.Filename,
						ChannelID:      dc.ChannelID,
						BotID:          res.BotID,
					}
					mu.Unlock()
					sent.Add(int64(len(data)))
					report()
				}
			}
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	base := filepath.Base(job.RemoteName)
	mimeType := mime.TypeByExtension(filepath.Ext(base))
	return &FileMeta{
		ID:           uuid.NewString(),
		LogicalPath:  strings.ReplaceAll(job.RemoteName, "\\", "/"),
		OriginalName: base,
		Size:         job.Size,
		MIMEType:     mimeType,
		ChunkCount:   nChunks,
		SHA256:       digest,
		UploadedAt:   time.Now().UTC(),
		Chunks:       chunks,
	}, nil
}

type atomicInt64 struct {
	mu sync.Mutex
	v  int64
}

func (a *atomicInt64) Add(n int64) {
	a.mu.Lock()
	a.v += n
	a.mu.Unlock()
}

func (a *atomicInt64) Load() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.v
}

// EnsureOutPathAvailable returns error if path exists and force is false.
func EnsureOutPathAvailable(path string, force bool) error {
	_, err := os.Stat(path)
	if err == nil {
		if !force {
			return fmt.Errorf("output exists: %s (use --force or confirm overwrite)", path)
		}
		return nil
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
