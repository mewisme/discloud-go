package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/selfmanaged"
	"github.com/mewisme/discloud-go/internal/transfer"
	"github.com/ncruces/zenity"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var name string
	var workers int
	var depth int
	var abort bool
	var mode string
	var dataFile string
	var force bool
	var usePassword bool
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "upload [path...]",
		Short: "Upload files (server-managed or self-managed Discord)",
		Args:  cobra.ArbitraryArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if abort {
				path, err := promptArg(ui.ArgOrEmpty(args, 0), "Path")
				if err != nil {
					return err
				}
				if len(args) > 1 {
					return fmt.Errorf("--abort accepts a single path")
				}
				c, err := apiClient()
				if err != nil {
					return err
				}
				if err := c.AbortUpload(path); err != nil {
					return err
				}
				ui.PrintSuccess("Aborted upload session for %s", path)
				return nil
			}

			if depth < 0 {
				return fmt.Errorf("--depth must be >= 0 (0=unlimited)")
			}

			paths := args
			if len(paths) == 0 {
				var err error
				paths, err = promptUploadPaths()
				if err != nil {
					return err
				}
			}

			jobs, err := expandUploadPaths(paths, depth)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return fmt.Errorf("no non-empty files to upload")
			}
			if name != "" && len(jobs) != 1 {
				return fmt.Errorf("--name only allowed with a single file")
			}
			if name != "" {
				jobs[0].remoteName = name
			}

			resolvedMode, err := resolveUploadMode(mode)
			if err != nil {
				return err
			}
			if resolvedMode == "self" {
				return uploadSelf(jobs, workers, dataFile, force, usePassword, passwordStdin)
			}

			c, err := apiClient()
			if err != nil {
				return err
			}
			return uploadJobs(c, jobs, workers)
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "remote file name (single file only)")
	cmd.Flags().BoolVar(&abort, "abort", false, "cancel server session for this path's checkpoint")
	cmd.Flags().StringVar(&mode, "mode", "", "upload mode: server|self")
	cmd.Flags().StringVar(&dataFile, "data-file", "", "output .discloud path (self mode)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .discloud without prompting")
	cmd.Flags().BoolVar(&usePassword, "password", false, "password-protect the .discloud bundle (self mode)")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read bundle password from stdin")
	cmd.PersistentFlags().IntVar(&workers, "workers", 0, "parallel chunk workers per file (0=auto: one per bot token in self mode)")
	cmd.PersistentFlags().IntVar(&depth, "depth", 0, "max folder walk depth (0=unlimited; 1=top-level files only)")
	cmd.AddCommand(newUploadPickCmd(&workers, &depth))
	return cmd
}

func resolveUploadMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "server", "self":
		return mode, nil
	case "":
		// continue
	default:
		return "", fmt.Errorf("invalid --mode %q (want server or self)", mode)
	}
	su, _ := client.LoadSelfUpload()
	interactive := !flagJSON && ui.IsTTY(os.Stdin)
	if interactive && su.Configured() {
		var pick string
		if err := huh.NewSelect[string]().
			Title("Select upload mode").
			Options(
				huh.NewOption("Server upload", "server"),
				huh.NewOption("Self upload", "self"),
			).
			Value(&pick).
			Run(); err != nil {
			return "", err
		}
		return pick, nil
	}
	return "server", nil
}

func uploadSelf(jobs []uploadJob, workers int, dataFile string, force, usePassword, passwordStdin bool) error {
	su, err := client.LoadSelfUpload()
	if err != nil {
		return err
	}
	if !su.Configured() {
		if !flagJSON && ui.IsTTY(os.Stdin) {
			ui.PrintInfo("Self-upload is not configured. Run: discloud config set self-upload")
		}
		return fmt.Errorf("self-upload is not configured")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	dc := discord.NewFromTokens(su.BotTokens, su.ChannelID)
	tier, err := dc.GuildPremiumTier(ctx, su.GuildID)
	if err != nil {
		return err
	}
	if err := dc.ValidateChannel(ctx); err != nil {
		return err
	}

	interactive := !flagJSON && ui.IsTTY(os.Stdin)
	adj := transfer.ResolveChunkSize(su.ChunkSize, su.PremiumTier, tier, interactive)
	chunkSize := adj.EffectiveSize
	if adj.Downgraded {
		fmt.Fprintf(os.Stderr, "The guild upload tier decreased from %d to %d.\nChunk size was reduced from %s to %s.\n",
			adj.OldTier, adj.Tier, transfer.FormatChunkMB(adj.OldSize), transfer.FormatChunkMB(chunkSize))
	}
	if adj.NeedsUpgrade {
		var choice string
		if err := huh.NewSelect[string]().
			Title(fmt.Sprintf("The guild now supports chunks up to %s.\nCurrent chunk size is %s.\nIncrease chunk size?",
				transfer.FormatChunkMB(adj.MaxChunk), transfer.FormatChunkMB(chunkSize))).
			Options(
				huh.NewOption("Keep "+transfer.FormatChunkMB(chunkSize), "keep"),
				huh.NewOption("Use maximum "+transfer.FormatChunkMB(adj.MaxChunk), "max"),
				huh.NewOption("Choose another size", "custom"),
			).
			Value(&choice).
			Run(); err != nil {
			return err
		}
		switch choice {
		case "max":
			chunkSize = adj.MaxChunk
		case "custom":
			var s string
			if err := huh.NewInput().Title("Chunk size in MB").Value(&s).Run(); err != nil {
				return err
			}
			var mb int
			if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &mb); err != nil || mb <= 0 {
				return fmt.Errorf("invalid chunk size")
			}
			want := transfer.MBToBytes(mb)
			if want > adj.MaxChunk {
				return fmt.Errorf("chunk size exceeds tier maximum %s", transfer.FormatChunkMB(adj.MaxChunk))
			}
			chunkSize = want
		}
	}
	if adj.InvalidFixed && interactive && !adj.Downgraded {
		ui.PrintInfo("Chunk size was invalid; using %s", transfer.FormatChunkMB(chunkSize))
	}
	_ = client.UpdateSelfUploadTierAndChunk(tier, chunkSize)

	out := dataFile
	if out == "" {
		base := ""
		if len(jobs) == 1 {
			base = filepath.Base(jobs[0].remoteName)
		}
		out = selfmanaged.DefaultBundleName(len(jobs), base)
		if interactive {
			out, err = promptBundleOutPath(out)
			if err != nil {
				return err
			}
		}
	} else if fixed, changed := selfmanaged.EnsureDiscloudExt(out); changed {
		out = fixed
	}
	if err := selfmanaged.EnsureOutPathAvailable(out, force); err != nil {
		if interactive && !force {
			if err2 := promptConfirm(fmt.Sprintf("Overwrite %s?", out)); err2 != nil {
				return err
			}
		} else {
			return err
		}
	}

	var password string
	if usePassword || passwordStdin {
		if passwordStdin {
			password, err = readPasswordStdin(os.Stdin)
			if err != nil {
				return err
			}
		} else if interactive {
			password, err = promptPassword("Password")
			if err != nil {
				return err
			}
			confirm, err := promptPassword("Confirm password")
			if err != nil {
				return err
			}
			if password != confirm {
				return fmt.Errorf("passwords do not match")
			}
		} else {
			return fmt.Errorf("password required: use --password-stdin or a TTY")
		}
		if err := selfmanaged.ValidatePassword(password); err != nil {
			return err
		}
	}

	var total int64
	for _, j := range jobs {
		total += j.size
	}
	tokens := client.NormalizeBotTokens(su.BotTokens)
	workersN := selfmanaged.ResolveWorkers(workers, len(tokens), 0)
	if !flagJSON {
		prot := selfmanaged.ProtectionObfuscated
		if password != "" {
			prot = selfmanaged.ProtectionPassword
		}
		_ = ui.PrintKVBlocks(os.Stderr, []ui.KVBlock{{
			Title: "Self upload",
			Rows: [][]string{
				{"mode", "self"},
				{"files", fmt.Sprintf("%d", len(jobs))},
				{"total size", client.FormatBytes(total)},
				{"premium tier", fmt.Sprintf("%d", tier)},
				{"max chunk", transfer.FormatChunkMB(transfer.MaxChunkForTier(tier))},
				{"chunk size", transfer.FormatChunkMB(chunkSize)},
				{"bot tokens", fmt.Sprintf("%d", len(tokens))},
				{"workers", fmt.Sprintf("%d", workersN)},
				{"bundle", out},
				{"protection", prot},
			},
		}})
		if password == "" {
			fmt.Fprintln(os.Stderr, "This bundle is obfuscated and integrity-protected. It is not securely encrypted because any compatible DisCloud CLI build can decode it.")
		}
	}

	sjobs := make([]selfmanaged.UploadJob, len(jobs))
	for i, j := range jobs {
		sjobs[i] = selfmanaged.UploadJob{Path: j.path, RemoteName: j.remoteName, Size: j.size}
	}

	showUI := ui.ShowWaitUI()
	var prog *ui.UploadProgress
	bars := make([]*ui.FileBar, len(jobs))
	if showUI {
		files := make([]ui.ProgressFile, len(jobs))
		for i, job := range jobs {
			files[i] = ui.ProgressFile{Name: job.remoteName, Total: job.size}
		}
		prog = ui.NewUploadProgress(os.Stderr, files)
		ctx = prog.Context()
		for i := range jobs {
			bars[i] = prog.Bar(i)
		}
	}

	opt := selfmanaged.UploadOptions{
		Jobs:        sjobs,
		OutPath:     out,
		Password:    password,
		GuildID:     su.GuildID,
		ChannelID:   su.ChannelID,
		BotTokens:   tokens,
		ChunkSize:   chunkSize,
		PremiumTier: tier,
		Workers:     workers,
		Progress: func(fileIdx int, sent, total int64) {
			if fileIdx >= 0 && fileIdx < len(bars) && bars[fileIdx] != nil {
				bars[fileIdx].Update(sent, total)
			}
		},
	}
	res, err := selfmanaged.Upload(ctx, opt)
	if prog != nil {
		for _, b := range bars {
			if b != nil {
				if err == nil {
					b.Done()
				} else {
					b.Abort()
				}
			}
		}
		prog.Wait()
	}
	if err != nil {
		if isUserAbort(err) {
			return nil
		}
		return err
	}
	if flagJSON {
		return writeJSON(map[string]any{
			"mode":        "self",
			"bundleId":    res.BundleID,
			"files":       res.Files,
			"totalSize":   res.TotalSize,
			"premiumTier": res.PremiumTier,
			"chunkSize":   res.ChunkSize,
			"protection":  res.Protection,
			"workers":     res.Workers,
			"dataFile":    res.DataFile,
		})
	}
	ui.PrintSuccess("Wrote %s (%d files, %s)", res.DataFile, res.Files, client.FormatBytes(res.TotalSize))
	return nil
}

func newUploadPickCmd(workers, depth *int) *cobra.Command {
	return &cobra.Command{
		Use:   "pick",
		Short: "Pick files or a folder in a dialog, then upload",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if *depth < 0 {
				return fmt.Errorf("--depth must be >= 0 (0=unlimited)")
			}
			pickMode, err := promptUploadPickMode()
			if err != nil {
				return err
			}

			var jobs []uploadJob
			switch pickMode {
			case "files":
				paths, err := pickUploadFiles()
				if errors.Is(err, zenity.ErrCanceled) {
					return nil
				}
				if err != nil {
					return err
				}
				jobs, err = expandUploadPaths(paths, *depth)
				if err != nil {
					return err
				}
			case "folder":
				dir, err := pickUploadFolder()
				if errors.Is(err, zenity.ErrCanceled) {
					return nil
				}
				if err != nil {
					return err
				}
				jobs, err = expandFolderJobs(dir, *depth)
				if err != nil {
					return err
				}
				if len(jobs) == 0 {
					return fmt.Errorf("no non-empty files in folder")
				}
				entries := make([]ui.UploadFile, len(jobs))
				for i, j := range jobs {
					entries[i] = ui.UploadFile{Path: j.path, Name: j.remoteName, Size: j.size}
				}
				selected, err := ui.SelectUploadFiles(entries)
				if err != nil {
					return err
				}
				jobs = make([]uploadJob, len(selected))
				for i, s := range selected {
					jobs[i] = uploadJob{path: s.Path, remoteName: s.Name, size: s.Size}
				}
			default:
				return fmt.Errorf("unknown pick mode %q", pickMode)
			}

			if len(jobs) == 0 {
				return fmt.Errorf("no non-empty files to upload")
			}
			mode, err := resolveUploadMode("")
			if err != nil {
				return err
			}
			if mode == "self" {
				return uploadSelf(jobs, *workers, "", false, false, false)
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			return uploadJobs(c, jobs, *workers)
		}),
	}
}

type uploadJob struct {
	path       string
	remoteName string
	size       int64
}

// promptBundleOutPath asks for the .discloud output path. Missing or truncated
// .discloud suffix is auto-fixed; user must confirm the updated name or re-edit.
func promptBundleOutPath(initial string) (string, error) {
	out := initial
	for {
		if err := huh.NewInput().
			Title("Output .discloud path").
			Value(&out).
			Validate(func(s string) error {
				if strings.TrimSpace(s) == "" {
					return fmt.Errorf("path is required")
				}
				return nil
			}).
			Run(); err != nil {
			return "", err
		}
		entered := strings.TrimSpace(out)
		fixed, changed := selfmanaged.EnsureDiscloudExt(entered)
		if !changed {
			return fixed, nil
		}
		var ok bool
		if err := huh.NewConfirm().
			Title(fmt.Sprintf("Path updated to %s. Continue?", fixed)).
			Affirmative("Yes").
			Negative("No").
			Value(&ok).
			Run(); err != nil {
			return "", err
		}
		if ok {
			return fixed, nil
		}
		out = entered
	}
}

func acceptUploadPath(paths []string, line string) (out []string, empty bool, err error) {
	parts := splitCommaFields(line)
	if len(parts) == 0 {
		return paths, true, nil
	}
	out = paths
	for _, p := range parts {
		if _, err := os.Stat(p); err != nil {
			return paths, false, err
		}
		out = append(out, p)
	}
	return out, false, nil
}

func promptUploadPaths() ([]string, error) {
	if flagJSON {
		return nil, fmt.Errorf("path is required with --json")
	}
	if !ui.IsTTY(os.Stdin) {
		return nil, fmt.Errorf("path is required")
	}
	var paths []string
	emptyStreak := 0
	for {
		var v string
		n := len(paths) + 1
		err := huh.NewInput().
			Title(fmt.Sprintf("Path #%d", n)).
			Description("Enter a file or folder path (comma-separated ok). Empty twice to start upload.").
			Value(&v).
			Validate(func(s string) error {
				parts := splitCommaFields(s)
				if len(parts) == 0 {
					return nil
				}
				for _, p := range parts {
					if _, err := os.Stat(p); err != nil {
						return err
					}
				}
				return nil
			}).
			Run()
		if err != nil {
			return nil, err
		}
		var empty bool
		paths, empty, err = acceptUploadPath(paths, v)
		if err != nil {
			return nil, err
		}
		if empty {
			emptyStreak++
			if emptyStreak >= 2 {
				break
			}
			continue
		}
		emptyStreak = 0
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("at least one path is required")
	}
	return paths, nil
}

func expandUploadPaths(paths []string, depth int) ([]uploadJob, error) {
	var jobs []uploadJob
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if st.Size() == 0 {
				continue
			}
			jobs = append(jobs, uploadJob{path: p, remoteName: filepath.Base(p), size: st.Size()})
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if skip, err := folderWalkSkip(p, path, d, depth); skip {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				return nil
			}
			rel, err := filepath.Rel(p, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "../") {
				return fmt.Errorf("invalid relative path %s", rel)
			}
			jobs = append(jobs, uploadJob{path: path, remoteName: rel, size: info.Size()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func uploadJobs(c *client.Client, jobs []uploadJob, workers int) error {
	showUI := ui.ShowWaitUI()
	var prog *ui.UploadProgress
	bars := make([]*ui.FileBar, len(jobs))

	ctx := context.Background()
	if showUI {
		files := make([]ui.ProgressFile, len(jobs))
		for i, job := range jobs {
			files[i] = ui.ProgressFile{Name: job.remoteName, Total: job.size}
		}
		prog = ui.NewUploadProgress(os.Stderr, files)
		ctx = prog.Context()
		for i := range jobs {
			bars[i] = prog.Bar(i)
		}
	} else {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
	}

	items := make([]FileItem, 0, len(jobs))
	var firstErr error
	for i, job := range jobs {
		if err := ctx.Err(); err != nil {
			firstErr = err
			for j := i; j < len(bars); j++ {
				if bars[j] != nil {
					bars[j].Abort()
				}
			}
			break
		}
		item, err := uploadOne(ctx, c, job.path, job.remoteName, workers, bars[i])
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", job.remoteName, err)
			}
			for j := i + 1; j < len(bars); j++ {
				if bars[j] != nil {
					bars[j].Abort()
				}
			}
			break
		}
		items = append(items, item)
	}
	canceled := ctx.Err() != nil
	if prog != nil {
		prog.Wait()
	}
	if canceled || isUserAbort(firstErr) {
		return nil
	}
	if firstErr != nil {
		return firstErr
	}

	if flagJSON {
		return writeJSON(items)
	}
	return printUploadSummary(items)
}

func uploadOne(ctx context.Context, c *client.Client, path, name string, workers int, bar *ui.FileBar) (FileItem, error) {
	var progress client.ProgressFunc
	if bar != nil {
		progress = bar.Update
	}
	raw, err := c.UploadChunked(path, client.UploadChunkedOptions{
		FileName: name,
		Workers:  workers,
		Progress: progress,
		Context:  ctx,
	})
	if err != nil {
		if bar != nil {
			bar.Abort()
		}
		return FileItem{}, err
	}
	if bar != nil {
		bar.Done()
	}
	return decode[FileItem](raw)
}

func printUploadSummary(items []FileItem) error {
	rows := make([]ui.UploadRow, len(items))
	for i, item := range items {
		rows[i] = ui.UploadRow{
			Name:        item.FileName,
			Size:        item.FileSize,
			Status:      item.Status,
			ID:          item.FileID,
			SHA256:      item.SHA256,
			AccessToken: item.AccessToken,
		}
	}
	return ui.PrintUploadTable(os.Stdout, rows)
}
