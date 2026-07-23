package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var name string
	var workers int
	var abort bool
	cmd := &cobra.Command{
		Use:   "upload [path]",
		Short: "Resumable chunked upload (session + local checkpoint)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path, err := ui.ResolveArg(ui.ArgOrEmpty(args, 0), "Path: ")
			if err != nil {
				return err
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			if abort {
				if err := c.AbortUpload(path); err != nil {
					return err
				}
				ui.PrintSuccess("Aborted upload session for %s", path)
				return nil
			}
			st, err := os.Stat(path)
			if err != nil {
				return err
			}
			if st.IsDir() {
				return uploadDir(c, path, workers)
			}
			return uploadOne(c, path, name, workers)
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "remote file name (or relative path)")
	cmd.Flags().IntVar(&workers, "workers", 0, "parallel chunk workers")
	cmd.Flags().BoolVar(&abort, "abort", false, "cancel server session for this path's checkpoint")
	return cmd
}

func uploadOne(c *client.Client, path, name string, workers int) error {
	var bar *ui.ProgressBar
	var progress client.ProgressFunc
	if ui.ShowWaitUI() {
		bar = ui.NewProgressBar(os.Stderr)
		progress = bar.Update
	}
	raw, err := c.UploadChunked(path, client.UploadChunkedOptions{
		FileName: name,
		Workers:  workers,
		Progress: progress,
	})
	if err != nil {
		if bar != nil {
			fmt.Fprintln(os.Stderr)
		}
		return err
	}
	if bar != nil {
		bar.Finish()
	}
	item, err := decode[FileItem](raw)
	if err != nil {
		return err
	}
	if flagJSON {
		return writeJSON(item)
	}
	printUploadSuccess(item)
	return nil
}

func uploadDir(c *client.Client, root string, workers int) error {
	var n int
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") {
			return fmt.Errorf("invalid relative path %s", rel)
		}
		ui.PrintSuccess("Uploading %s", rel)
		if err := uploadOne(c, path, rel, workers); err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		n++
		return nil
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no non-empty files under %s", root)
	}
	return nil
}

func printUploadSuccess(item FileItem) {
	on := ui.ColorOn(os.Stdout)
	status := item.Status
	if status == "" {
		status = "ready"
	}
	ui.PrintSuccess("%s %s (%s) %s %s",
		ui.IconUp,
		ui.Bold(on, item.FileName),
		ui.Dim(on, client.FormatBytes(item.FileSize)),
		ui.Cyan(on, item.FileID),
		ui.Dim(on, status),
	)
	if item.SHA256 != "" {
		fmt.Printf("%s sha256 %s\n", ui.Dim(on, "·"), ui.Cyan(on, item.SHA256))
	}
	if item.AccessToken != "" {
		fmt.Printf("%s %s\n", ui.Yellow(on, ui.IconKey), ui.Cyan(on, item.AccessToken))
	}
}

func newUploadRawCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "upload-raw [path]",
		Short: "Single POST /api/upload",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path, err := ui.ResolveArg(ui.ArgOrEmpty(args, 0), "Path: ")
			if err != nil {
				return err
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Uploading…", func() (map[string]any, error) {
				return c.UploadRaw(path, name)
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
			printUploadSuccess(item)
			return nil
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "remote file name")
	return cmd
}
