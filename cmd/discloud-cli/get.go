package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/selfmanaged"
	"github.com/ncruces/zenity"
	"github.com/spf13/cobra"
)

type getFlags struct {
	name, token, password, out string
	download, meta             bool
	passwordStdin              bool
	allFiles                   bool
	fileSelectors              []string
	force                      bool
	pickMode                   bool
}

func newGetCmd() *cobra.Command {
	var f getFlags
	cmd := &cobra.Command{
		Use:   "get [id-or-bundle]",
		Short: "Download a server file or a local .discloud bundle",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			return runGet(ui.ArgOrEmpty(args, 0), f)
		}),
	}
	bindGetFlags(cmd, &f)
	cmd.AddCommand(newGetPickCmd(&f))
	return cmd
}

func bindGetFlags(cmd *cobra.Command, f *getFlags) {
	cmd.Flags().StringVar(&f.name, "name", "", "name path segment")
	cmd.Flags().StringVar(&f.token, "token", "", "access token for private files")
	cmd.Flags().StringVar(&f.password, "password", "", "share password or bundle decryption password")
	cmd.Flags().BoolVar(&f.passwordStdin, "password-stdin", false, "read bundle password from stdin")
	cmd.Flags().StringVar(&f.out, "out", "", "write body to this path (file or directory)")
	cmd.Flags().BoolVar(&f.download, "download", false, "request download disposition / extend retention")
	cmd.Flags().BoolVar(&f.meta, "meta", false, "fetch JSON metadata from API (?json=1)")
	cmd.Flags().BoolVar(&f.allFiles, "all", false, "download all files from a multi-file bundle")
	cmd.Flags().StringArrayVar(&f.fileSelectors, "file", nil, "select bundle file by id or name (repeatable)")
	cmd.Flags().BoolVar(&f.force, "force", false, "overwrite existing download paths without prompting")
}

func newGetPickCmd(f *getFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pick [id-or-bundle]",
		Short: "Pick .discloud file and/or download folder, then download",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			arg := ui.ArgOrEmpty(args, 0)
			if arg == "" {
				path, err := pickBundleFile()
				if errors.Is(err, zenity.ErrCanceled) {
					return nil
				}
				if err != nil {
					return err
				}
				arg = path
			}
			dir, err := pickDownloadDir()
			if errors.Is(err, zenity.ErrCanceled) {
				return nil
			}
			if err != nil {
				return err
			}
			f.out = dir
			f.pickMode = true
			return runGet(arg, *f)
		}),
	}
	bindGetFlags(cmd, f)
	return cmd
}

func runGet(arg string, f getFlags) error {
	if arg != "" {
		if st, err := os.Stat(arg); err == nil && !st.IsDir() && selfmanaged.DetectBundle(arg) {
			return getSelfBundle(arg, f)
		}
	}

	c, err := apiClient()
	if err != nil {
		return err
	}
	id, err := resolveFileID(c, arg)
	if err != nil {
		return err
	}
	wantMeta := f.meta || (flagJSON && !f.download && f.out == "")
	outPath := f.out
	if outPath == "" && f.download && !wantMeta {
		outPath = id
		if f.name != "" {
			outPath = filepath.Base(f.name)
		}
	}
	outPath = resolveServerOutPath(outPath, id, f.name)

	if wantMeta {
		data, err := ui.WaitVal("Fetching metadata…", func() ([]byte, error) {
			return c.Download(id, client.DownloadOptions{
				Name: f.name, Download: f.download, JSON: true, Token: f.token, Password: f.password, OutPath: outPath,
			})
		})
		if err != nil {
			return err
		}
		if outPath != "" {
			if flagJSON {
				return writeJSON(map[string]string{"path": outPath})
			}
			ui.PrintSuccess("wrote %s", outPath)
			return nil
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
		if len(data) > 0 && data[len(data)-1] != '\n' {
			fmt.Println()
		}
		return nil
	}

	opt := client.DownloadOptions{
		Name: f.name, Download: f.download, JSON: false, Token: f.token, Password: f.password, OutPath: outPath,
	}
	showUI := ui.ShowWaitUI()
	var prog *ui.UploadProgress
	var bar *ui.FileBar
	if showUI {
		name := id
		if f.name != "" {
			name = filepath.Base(f.name)
		} else if outPath != "" {
			name = filepath.Base(outPath)
		}
		prog = ui.NewDownloadProgress(os.Stderr, []ui.ProgressFile{{Name: name, Total: 0}})
		bar = prog.Bar(0)
		opt.Context = prog.Context()
		opt.Progress = func(sent, total int64) {
			if bar != nil {
				bar.Update(sent, total)
			}
		}
	}
	data, err := c.Download(id, opt)
	if prog != nil {
		if bar != nil {
			if err == nil {
				bar.Done()
			} else {
				bar.Abort()
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
	if outPath != "" {
		if flagJSON {
			return writeJSON(map[string]string{"path": outPath})
		}
		ui.PrintSuccess("wrote %s", outPath)
		return nil
	}
	if _, err := os.Stdout.Write(data); err != nil {
		return err
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

// resolveServerOutPath joins a directory --out with the download filename.
func resolveServerOutPath(out, id, name string) string {
	if out == "" {
		return ""
	}
	st, err := os.Stat(out)
	if err != nil || !st.IsDir() {
		return out
	}
	fname := id
	if name != "" {
		fname = filepath.Base(name)
	}
	return filepath.Join(out, fname)
}

func getSelfBundle(path string, f getFlags) error {
	meta, password, err := loadBundleMeta(path, f.password, f.passwordStdin)
	if err != nil {
		return err
	}
	_ = password // retained for rewrite paths; download only needs meta

	su, err := client.LoadSelfUpload()
	if err != nil {
		return err
	}
	if !su.Configured() {
		return fmt.Errorf("self-upload is not configured (required to download from Discord)")
	}

	selected, err := selectBundleFiles(meta.Files, f.fileSelectors, f.allFiles, f.pickMode)
	if err != nil {
		return err
	}

	outPath := f.out
	if outPath == "" {
		if !flagJSON && ui.IsTTY(os.Stdin) {
			outPath = "."
			if err := huh.NewInput().Title("Output path").Value(&outPath).Run(); err != nil {
				return err
			}
		} else {
			outPath = "."
		}
	}

	selected, err = resolveDownloadConflicts(outPath, selected, f.force)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	showUI := ui.ShowWaitUI()
	var prog *ui.UploadProgress
	bars := make([]*ui.FileBar, len(selected))
	if showUI {
		files := make([]ui.ProgressFile, len(selected))
		for i, file := range selected {
			name := file.LogicalPath
			if name == "" {
				name = file.OriginalName
			}
			files[i] = ui.ProgressFile{Name: name, Total: file.Size}
		}
		prog = ui.NewDownloadProgress(os.Stderr, files)
		ctx = prog.Context()
		for i := range selected {
			bars[i] = prog.Bar(i)
		}
	}

	res, err := selfmanaged.Download(ctx, selfmanaged.DownloadOptions{
		Meta:            meta,
		Files:           selected,
		OutPath:         outPath,
		BotTokens:       su.BotTokens,
		ChannelID:       su.ChannelID,
		GuildID:         su.GuildID,
		RequireIdentity: true,
		Progress: func(fileIdx int, sent, total int64) {
			if fileIdx >= 0 && fileIdx < len(bars) && bars[fileIdx] != nil {
				bars[fileIdx].Update(sent, total)
			}
		},
	})
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
			"mode":  "self",
			"paths": res.Paths,
		})
	}
	for _, p := range res.Paths {
		ui.PrintSuccess("wrote %s", p)
	}
	return nil
}

func loadBundleMeta(path, password string, passwordStdin bool) (*selfmanaged.BundleMeta, string, error) {
	mode, err := selfmanaged.DecodePeekMode(path)
	if err != nil {
		return nil, "", err
	}
	pw := password
	if mode == selfmanaged.ProtectionPassword {
		if passwordStdin {
			pw, err = readPasswordStdin(os.Stdin)
			if err != nil {
				return nil, "", err
			}
		} else if pw == "" {
			if flagJSON || !ui.IsTTY(os.Stdin) {
				return nil, "", fmt.Errorf("password required: use --password, --password-stdin, or a TTY")
			}
			pw, err = promptPassword("Password")
			if err != nil {
				return nil, "", err
			}
		}
	}
	meta, err := selfmanaged.Decode(path, selfmanaged.DecodeOptions{Password: pw})
	if err != nil {
		return nil, "", err
	}
	return meta, pw, nil
}

func selectBundleFiles(files []selfmanaged.FileMeta, selectors []string, all, pickMode bool) ([]selfmanaged.FileMeta, error) {
	if all || len(selectors) > 0 {
		return selfmanaged.SelectFiles(files, selectors, all)
	}
	if !pickMode && len(files) <= 1 {
		return selfmanaged.SelectFiles(files, nil, true)
	}
	if flagJSON || !ui.IsTTY(os.Stdin) || !ui.IsTTY(os.Stdout) {
		return nil, fmt.Errorf("multiple files: pass --file, --all, or run interactively")
	}
	entries := make([]ui.FileEntry, len(files))
	for i, f := range files {
		name := f.LogicalPath
		if name == "" {
			name = f.OriginalName
		}
		entries[i] = ui.FileEntry{Path: f.ID, Name: name, Size: f.Size}
	}
	selected, err := ui.SelectDownloadFiles(entries)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(selected))
	for i, s := range selected {
		ids[i] = s.Path
	}
	return selfmanaged.SelectFiles(files, ids, false)
}

// existingDownloadPaths returns dest paths under outDir that already exist as files.
func existingDownloadPaths(outDir string, files []selfmanaged.FileMeta) []string {
	var conflicts []string
	for _, f := range files {
		logical := f.LogicalPath
		if logical == "" {
			logical = f.OriginalName
		}
		dest, err := selfmanaged.SafeJoin(outDir, logical)
		if err != nil {
			continue
		}
		st, err := os.Stat(dest)
		if err == nil && !st.IsDir() {
			conflicts = append(conflicts, dest)
		}
	}
	return conflicts
}

// resolveDownloadConflicts asks to overwrite on conflict. Yes keeps paths;
// No renames to name.N.ext (N starts at 1, +1 while that path exists).
func resolveDownloadConflicts(outDir string, files []selfmanaged.FileMeta, force bool) ([]selfmanaged.FileMeta, error) {
	conflicts := existingDownloadPaths(outDir, files)
	if len(conflicts) == 0 {
		return files, nil
	}
	if force {
		return files, nil
	}
	if flagJSON || !ui.IsTTY(os.Stdin) {
		return nil, fmt.Errorf("refusing to overwrite %d existing file(s); pass --force", len(conflicts))
	}
	msg := fmt.Sprintf("Overwrite %d existing file(s)?", len(conflicts))
	if len(conflicts) <= 3 {
		msg = fmt.Sprintf("Overwrite existing file(s)?\n%s", strings.Join(conflicts, "\n"))
	}
	var overwrite bool
	if err := huh.NewConfirm().
		Title(msg).
		Affirmative("Overwrite").
		Negative("Keep both (rename)").
		Value(&overwrite).
		Run(); err != nil {
		return nil, err
	}
	if overwrite {
		return files, nil
	}
	return indexConflictingDownloads(outDir, files)
}

// indexConflictingDownloads sets LogicalPath to name.N.ext for files whose dest exists.
func indexConflictingDownloads(outDir string, files []selfmanaged.FileMeta) ([]selfmanaged.FileMeta, error) {
	out := make([]selfmanaged.FileMeta, len(files))
	copy(out, files)
	reserved := make(map[string]struct{}, len(files))
	for i := range out {
		logical := out[i].LogicalPath
		if logical == "" {
			logical = out[i].OriginalName
		}
		dest, err := selfmanaged.SafeJoin(outDir, logical)
		if err != nil {
			continue
		}
		st, err := os.Stat(dest)
		if err != nil || st.IsDir() {
			reserved[dest] = struct{}{}
			continue
		}
		indexed := nextIndexedDest(dest, reserved)
		reserved[indexed] = struct{}{}
		rel, err := filepath.Rel(outDir, indexed)
		if err != nil {
			return nil, err
		}
		out[i].LogicalPath = filepath.ToSlash(rel)
	}
	return out, nil
}

// nextIndexedDest returns dest with .N inserted before the extension (name.1.ext).
// N starts at 1 and increments while the candidate exists on disk or in reserved.
func nextIndexedDest(dest string, reserved map[string]struct{}) string {
	dir := filepath.Dir(dest)
	base := filepath.Base(dest)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for n := 1; ; n++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s.%d%s", stem, n, ext))
		if _, err := os.Stat(candidate); err == nil {
			continue
		}
		if _, ok := reserved[candidate]; ok {
			continue
		}
		return candidate
	}
}
