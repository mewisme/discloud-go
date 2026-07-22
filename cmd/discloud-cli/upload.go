package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var name string
	var workers int
	cmd := &cobra.Command{
		Use:   "upload [path]",
		Short: "Resumable chunked upload",
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
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "remote file name")
	cmd.Flags().IntVar(&workers, "workers", 0, "parallel chunk workers")
	return cmd
}

func printUploadSuccess(item FileItem) {
	on := ui.ColorOn(os.Stdout)
	ui.PrintSuccess("%s %s (%s) %s",
		ui.IconUp,
		ui.Bold(on, item.FileName),
		ui.Dim(on, client.FormatBytes(item.FileSize)),
		ui.Cyan(on, item.FileID),
	)
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
