package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var name string
	var workers int
	cmd := &cobra.Command{
		Use:   "upload <path>",
		Short: "Resumable chunked upload",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			var bar *progressBar
			var progress client.ProgressFunc
			if showWaitUI() {
				bar = newProgressBar(os.Stderr)
				progress = bar.Update
			}
			raw, err := c.UploadChunked(args[0], client.UploadChunkedOptions{
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
	on := colorOn(os.Stdout)
	printSuccess("%s %s (%s) %s",
		iconUp,
		bold(on, item.FileName),
		dim(on, client.FormatBytes(item.FileSize)),
		cyan(on, item.FileID),
	)
	if item.AccessToken != "" {
		fmt.Printf("%s %s\n", yellow(on, iconKey), cyan(on, item.AccessToken))
	}
}

func newUploadRawCmd() *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "upload-raw <path>",
		Short: "Single POST /api/upload",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := waitVal("Uploading…", func() (map[string]any, error) {
				return c.UploadRaw(args[0], name)
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
