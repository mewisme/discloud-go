package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/integrity"
	"github.com/spf13/cobra"
)

func newFilesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "files",
		Short: "Manage owned files",
	}
	cmd.AddCommand(
		newFilesListCmd(),
		newFilesGetCmd(),
		newFilesInspectCmd(),
		newFilesVerifyCmd(),
		newFilesVisibilityCmd(),
		newFilesShareCmd(),
		newFilesRevokeCmd(),
		newFilesRotateTokenCmd(),
		newFilesDeleteCmd(),
	)
	return cmd
}

func newFilesListCmd() *cobra.Command {
	var limit, offset int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List owned files",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Loading files…", func() (map[string]any, error) {
				return c.ListFiles(limit, offset)
			})
			if err != nil {
				return err
			}
			list, err := decode[FilesList](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(list)
			}
			if len(list.Files) == 0 {
				ui.PrintInfo("No files")
				return nil
			}
			return ui.PrintFileTable(os.Stdout, toUIFiles(list.Files))
		}),
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "page size")
	cmd.Flags().IntVar(&offset, "offset", 0, "page offset")
	return cmd
}

func newFilesGetCmd() *cobra.Command {
	var token string
	cmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Get file metadata",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Loading file…", func() (map[string]any, error) {
				return c.GetFile(id, token)
			})
			if err != nil {
				return err
			}
			item, err := decode[FileItem](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(item)
			}
			return ui.PrintFileTable(os.Stdout, toUIFiles([]FileItem{item}))
		}),
	}
	cmd.Flags().StringVar(&token, "token", "", "access token for private files")
	return cmd
}

func newFilesInspectCmd() *cobra.Command {
	var token string
	cmd := &cobra.Command{
		Use:   "inspect [id]",
		Short: "Inspect file analytics",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Inspecting…", func() (map[string]any, error) {
				return c.Inspect(id, token)
			})
			if err != nil {
				return err
			}
			item, err := decode[InspectResponse](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(item)
			}
			return ui.PrintInspect(os.Stdout, toUIInspect(item))
		}),
	}
	cmd.Flags().StringVar(&token, "token", "", "access token for private files")
	return cmd
}

func newFilesVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <path> <sha256>",
		Short: "Verify a local file against a discloud-sha256-v1 digest",
		Args:  cobra.ExactArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path := args[0]
			want := strings.TrimSpace(strings.ToLower(args[1]))
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()
			got, err := integrity.FileSHA256FromReader(f)
			if err != nil {
				return err
			}
			on := ui.ColorOn(os.Stdout)
			if !strings.EqualFold(got, want) {
				fmt.Fprintf(os.Stderr, "%s mismatch\n  got  %s\n  want %s\n",
					ui.Red(on, "verify failed"), got, want)
				return fmt.Errorf("digest mismatch")
			}
			ui.PrintSuccess("%s %s matches %s",
				ui.IconOK,
				ui.Bold(on, path),
				ui.Cyan(on, got),
			)
			return nil
		}),
	}
}

func newFilesVisibilityCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "visibility [id] {public|private}",
		Short: "Set file visibility",
		Args:  cobra.MaximumNArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			var id, vis string
			switch len(args) {
			case 0:
				var cur string
				id, cur, err = pickFileVisibility(c)
				if err != nil {
					return err
				}
				vis, err = resolveVisibility("", cur)
				if err != nil {
					return err
				}
			case 1:
				if args[0] == "public" || args[0] == "private" {
					vis = args[0]
					id, _, err = pickFileVisibility(c)
				} else {
					id = args[0]
					cur, err := currentVisibility(c, id)
					if err != nil {
						return err
					}
					vis, err = resolveVisibility("", cur)
				}
				if err != nil {
					return err
				}
			default:
				id = args[0]
				vis, err = resolveVisibility(args[1], "")
				if err != nil {
					return err
				}
			}

			if vis == "public" && !yes {
				cur, err := currentVisibility(c, id)
				if err != nil {
					return err
				}
				if cur == "private" {
					if err := promptConfirm(fmt.Sprintf("Make %s public? This invalidates the private token.", id)); err != nil {
						return err
					}
				}
			}

			raw, err := ui.WaitVal("Updating visibility…", func() (map[string]any, error) {
				return c.SetVisibility(id, vis)
			})
			if err != nil {
				return err
			}
			item, err := decode[FileItem](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(item)
			}
			visLabel := item.Visibility
			switch item.Visibility {
			case "private":
				visLabel = ui.IconLock + " private"
			case "public":
				visLabel = ui.IconUnlock + " public"
			}
			rows := [][]string{
				{ui.IconOK, "File", id},
				{ui.IconOK, "Visibility", visLabel},
			}
			if item.AccessToken != "" {
				rows = append(rows, []string{ui.IconOK, "Token", item.AccessToken})
			}
			return ui.PrintGlyphTable(os.Stdout, rows)
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newFilesShareCmd() *cobra.Command {
	var (
		password      string
		clearPassword bool
		expires       string
		maxDownloads  int
		clearMaxDL    bool
		mode          string
	)
	cmd := &cobra.Command{
		Use:   "share [id]",
		Short: "Update share settings (password, expiry, download cap, mode)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			patch := client.FileShareUpdate{}
			changed := false
			if clearPassword {
				empty := ""
				patch.Password = &empty
				changed = true
			} else if password != "" {
				patch.Password = &password
				changed = true
			}
			if expires != "" {
				patch.ExpiresAt = &expires
				changed = true
			}
			if clearMaxDL {
				zero := 0
				patch.MaxDownloads = &zero
				changed = true
			} else if cmd.Flags().Changed("max-downloads") {
				patch.MaxDownloads = &maxDownloads
				changed = true
			}
			if mode != "" {
				if mode != "view" && mode != "download" {
					return fmt.Errorf("mode must be view or download")
				}
				patch.ShareMode = &mode
				changed = true
			}
			if !changed {
				return fmt.Errorf("pass at least one of --password, --clear-password, --expires, --max-downloads, --clear-max-downloads, --mode")
			}
			raw, err := ui.WaitVal("Updating share…", func() (map[string]any, error) {
				return c.UpdateShare(id, patch)
			})
			if err != nil {
				return err
			}
			item, err := decode[FileItem](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(item)
			}
			rows := [][]string{
				{ui.IconOK, "File", item.FileID},
				{ui.IconOK, "Mode", item.ShareMode},
				{ui.IconOK, "Password", fmt.Sprintf("%v", item.PasswordProtected)},
				{ui.IconOK, "Expires", item.ExpiresAt.UTC().Format(time.RFC3339)},
			}
			if item.MaxDownloads != nil {
				rows = append(rows, []string{ui.IconOK, "Max downloads", fmt.Sprintf("%d", *item.MaxDownloads)})
			} else {
				rows = append(rows, []string{ui.IconOK, "Max downloads", "unlimited"})
			}
			rows = append(rows, []string{ui.IconOK, "Download count", fmt.Sprintf("%d", item.DownloadCount)})
			return ui.PrintGlyphTable(os.Stdout, rows)
		}),
	}
	cmd.Flags().StringVar(&password, "password", "", "set share password")
	cmd.Flags().BoolVar(&clearPassword, "clear-password", false, "remove share password")
	cmd.Flags().StringVar(&expires, "expires", "", "RFC3339 expiry (within 30 days)")
	cmd.Flags().IntVar(&maxDownloads, "max-downloads", 0, "download cap (use --clear-max-downloads for unlimited)")
	cmd.Flags().BoolVar(&clearMaxDL, "clear-max-downloads", false, "remove download cap")
	cmd.Flags().StringVar(&mode, "mode", "", "view or download")
	return cmd
}

func newFilesRevokeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke [id]",
		Short: "Force-expire a share without deleting the file row",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			if !yes {
				if err := promptConfirm(fmt.Sprintf("Revoke share for %s?", id)); err != nil {
					return err
				}
			}
			if err := ui.WithSpinner("Revoking…", func() error {
				return c.RevokeFile(id)
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]any{"revoked": true, "fileId": id})
			}
			on := ui.ColorOn(os.Stdout)
			ui.PrintSuccess("Revoked %s", ui.Cyan(on, id))
			return nil
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newFilesRotateTokenCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "rotate-token [id]",
		Short: "Rotate private-file access token",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			if !yes {
				if err := promptConfirm(fmt.Sprintf("Rotate access token for %s?", id)); err != nil {
					return err
				}
			}
			raw, err := ui.WaitVal("Rotating token…", func() (map[string]any, error) {
				return c.RotateToken(id)
			})
			if err != nil {
				return err
			}
			item, err := decode[FileItem](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(item)
			}
			on := ui.ColorOn(os.Stdout)
			ui.PrintSuccess("Rotated token for %s", ui.Cyan(on, item.FileID))
			if item.AccessToken != "" {
				fmt.Printf("%s %s\n", ui.Yellow(on, ui.IconKey), ui.Cyan(on, item.AccessToken))
			}
			return nil
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newFilesDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete [id]",
		Short: "Delete a file (metadata only)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, ui.ArgOrEmpty(args, 0))
			if err != nil {
				return err
			}
			if !yes {
				if err := promptConfirm(fmt.Sprintf("Delete %s?", id)); err != nil {
					return err
				}
			}
			if err := ui.WithSpinner("Deleting…", func() error {
				return c.DeleteFile(id)
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"deleted": id})
			}
			on := ui.ColorOn(os.Stdout)
			ui.PrintSuccess("Deleted %s", ui.Cyan(on, id))
			return nil
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func currentVisibility(c *client.Client, id string) (string, error) {
	raw, err := ui.WaitVal("Loading file…", func() (map[string]any, error) {
		return c.GetFile(id, "")
	})
	if err != nil {
		return "", err
	}
	item, err := decode[FileItem](raw)
	if err != nil {
		return "", err
	}
	return item.Visibility, nil
}

// resolveFileID returns id, or opens a TTY picker (disabled under --json / non-TTY).
func resolveFileID(c *client.Client, id string) (string, error) {
	if id != "" {
		return id, nil
	}
	if flagJSON {
		return "", fmt.Errorf("file id required with --json (interactive picker disabled)")
	}
	if !ui.IsTTY(os.Stdin) {
		return "", fmt.Errorf("file id required (interactive picker needs a TTY)")
	}
	files, err := fetchFilesForPicker(c)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files to select")
	}
	opts := make([]huh.Option[string], len(files))
	for i, f := range files {
		opts[i] = huh.NewOption(fmt.Sprintf("%s · %s", f.FileName, f.FileID), f.FileID)
	}
	var picked string
	if err := huh.NewSelect[string]().
		Title("Select file").
		Options(opts...).
		Value(&picked).
		Run(); err != nil {
		return "", err
	}
	return picked, nil
}

// pickFileVisibility opens the file picker and returns id + current visibility.
func pickFileVisibility(c *client.Client) (id, cur string, err error) {
	if flagJSON {
		return "", "", fmt.Errorf("file id required with --json (interactive picker disabled)")
	}
	if !ui.IsTTY(os.Stdin) {
		return "", "", fmt.Errorf("file id required (interactive picker needs a TTY)")
	}
	files, err := fetchFilesForPicker(c)
	if err != nil {
		return "", "", err
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("no files to select")
	}
	opts := make([]huh.Option[string], len(files))
	byID := make(map[string]string, len(files))
	for i, f := range files {
		opts[i] = huh.NewOption(fmt.Sprintf("%s · %s", f.FileName, f.FileID), f.FileID)
		byID[f.FileID] = f.Visibility
	}
	var picked string
	if err := huh.NewSelect[string]().
		Title("Select file").
		Options(opts...).
		Value(&picked).
		Run(); err != nil {
		return "", "", err
	}
	return picked, byID[picked], nil
}

// resolveVisibility returns public|private, or opens a Select when empty.
func resolveVisibility(arg, defaultVis string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "public" || arg == "private" {
		return arg, nil
	}
	if arg != "" {
		return "", fmt.Errorf("visibility must be public or private")
	}
	if flagJSON {
		return "", fmt.Errorf("visibility required with --json (interactive picker disabled)")
	}
	if !ui.IsTTY(os.Stdin) {
		return "", fmt.Errorf("visibility required (interactive picker needs a TTY)")
	}
	vis := defaultVis
	if vis != "public" && vis != "private" {
		vis = "private"
	}
	if err := huh.NewSelect[string]().
		Title("Visibility").
		Options(
			huh.NewOption("public", "public"),
			huh.NewOption("private", "private"),
		).
		Value(&vis).
		Run(); err != nil {
		return "", err
	}
	return vis, nil
}

func fetchFilesForPicker(c *client.Client) ([]FileItem, error) {
	return ui.WaitVal("Loading files…", func() ([]FileItem, error) {
		var all []FileItem
		offset := 0
		for {
			raw, err := c.ListFiles(pickerPageSize, offset)
			if err != nil {
				return nil, err
			}
			page, err := decode[FilesList](raw)
			if err != nil {
				return nil, err
			}
			if len(page.Files) == 0 {
				break
			}
			all = append(all, page.Files...)
			if len(all) > pickerSafetyLimit {
				return nil, fmt.Errorf(
					"more than %d files; pass an explicit id or use --limit/--offset on files list",
					pickerSafetyLimit,
				)
			}
			if len(page.Files) < pickerPageSize {
				break
			}
			offset += len(page.Files)
		}
		return all, nil
	})
}

func toUIFiles(files []FileItem) []ui.File {
	out := make([]ui.File, len(files))
	for i, f := range files {
		out[i] = ui.File{
			ID:         f.FileID,
			Name:       f.FileName,
			Size:       f.FileSize,
			Status:     f.Status,
			Visibility: f.Visibility,
			Expires:    f.ExpiresAt,
		}
	}
	return out
}

func toUIInspect(item InspectResponse) ui.Inspect {
	return ui.Inspect{
		FileID:            item.FileID,
		FileName:          item.FileName,
		FileSize:          item.FileSize,
		ChunkSize:         item.ChunkSize,
		ChunkCount:        item.ChunkCount,
		CreatedAt:         item.CreatedAt,
		ExpiresAt:         item.ExpiresAt,
		Visibility:        item.Visibility,
		Status:            item.Status,
		PasswordProtected: item.PasswordProtected,
		ShareMode:         item.ShareMode,
		MaxDownloads:      item.MaxDownloads,
		DownloadCount:     item.DownloadCount,
		SHA256:            item.SHA256,
		Views:             item.Views,
		Downloads:         item.Downloads,
		Ranges:            item.Ranges,
		BytesServed:       item.BytesServed,
		UniqueVisitors:    item.UniqueVisitors,
		LastAccessAt:      item.LastAccessAt,
		URL:               item.URL,
		LongURL:           item.LongURL,
		DownloadURL:       item.DownloadURL,
		LongDownloadURL:   item.LongDownloadURL,
	}
}
