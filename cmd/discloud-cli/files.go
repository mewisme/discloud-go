package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
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
		newFilesVisibilityCmd(),
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
				vis, err = ui.ResolveArg("", "Visibility (public|private): ")
				if err != nil {
					return err
				}
				id, err = resolveFileID(c, "")
				if err != nil {
					return err
				}
			case 1:
				vis = args[0]
				id, err = resolveFileID(c, "")
				if err != nil {
					return err
				}
			default:
				id = args[0]
				vis = args[1]
			}
			if vis != "public" && vis != "private" {
				return fmt.Errorf("visibility must be public or private")
			}

			if vis == "public" && !yes {
				cur, err := currentVisibility(c, id)
				if err != nil {
					return err
				}
				if cur == "private" {
					if !ui.IsTTY(os.Stdin) || flagJSON {
						return fmt.Errorf("refusing private→public without -y (non-interactive)")
					}
					if !ui.Confirm(os.Stderr, os.Stdin, fmt.Sprintf("Make %s public? This invalidates the private token.", id)) {
						return fmt.Errorf("aborted")
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
			on := ui.ColorOn(os.Stdout)
			ui.PrintSuccess("%s is now %s", ui.Cyan(on, item.FileID), ui.VisibilityLabel(on, item.Visibility))
			if item.AccessToken != "" {
				fmt.Printf("%s %s\n", ui.Yellow(on, ui.IconKey), ui.Cyan(on, item.AccessToken))
			}
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
				if !ui.IsTTY(os.Stdin) || flagJSON {
					return fmt.Errorf("refusing rotate-token without -y (non-interactive)")
				}
				if !ui.Confirm(os.Stderr, os.Stdin, fmt.Sprintf("Rotate access token for %s?", id)) {
					return fmt.Errorf("aborted")
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
				if !ui.IsTTY(os.Stdin) || flagJSON {
					return fmt.Errorf("refusing delete without -y (non-interactive)")
				}
				if !ui.Confirm(os.Stderr, os.Stdin, fmt.Sprintf("Delete %s?", id)) {
					return fmt.Errorf("aborted")
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
	idx, err := ui.PickFile(os.Stderr, os.Stdin, toUIFiles(files))
	if err != nil {
		return "", err
	}
	return files[idx].FileID, nil
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
			Visibility: f.Visibility,
			Expires:    f.ExpiresAt,
		}
	}
	return out
}

func toUIInspect(item InspectResponse) ui.Inspect {
	return ui.Inspect{
		FileID:          item.FileID,
		FileName:        item.FileName,
		FileSize:        item.FileSize,
		ChunkSize:       item.ChunkSize,
		ChunkCount:      item.ChunkCount,
		CreatedAt:       item.CreatedAt,
		ExpiresAt:       item.ExpiresAt,
		Visibility:      item.Visibility,
		Views:           item.Views,
		Downloads:       item.Downloads,
		Ranges:          item.Ranges,
		BytesServed:     item.BytesServed,
		UniqueVisitors:  item.UniqueVisitors,
		LastAccessAt:    item.LastAccessAt,
		URL:             item.URL,
		LongURL:         item.LongURL,
		DownloadURL:     item.DownloadURL,
		LongDownloadURL: item.LongDownloadURL,
	}
}
