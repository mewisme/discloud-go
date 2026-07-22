package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newGetCmd() *cobra.Command {
	var (
		name, token, out string
		download, meta   bool
	)
	cmd := &cobra.Command{
		Use:   "get [id]",
		Short: "Download a file (or metadata with --meta)",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			id, err := resolveFileID(c, argOrEmpty(args, 0))
			if err != nil {
				return err
			}
			wantMeta := meta || (flagJSON && !download && out == "")
			outPath := out
			if outPath == "" && download && !wantMeta {
				outPath = id
				if name != "" {
					outPath = filepath.Base(name)
				}
			}
			msg := "Downloading…"
			if wantMeta {
				msg = "Fetching metadata…"
			}
			data, err := waitVal(msg, func() ([]byte, error) {
				return c.Download(id, client.DownloadOptions{
					Name: name, Download: download, JSON: wantMeta, Token: token, OutPath: outPath,
				})
			})
			if err != nil {
				return err
			}
			if outPath != "" {
				if flagJSON {
					return writeJSON(map[string]string{"path": outPath})
				}
				printSuccess("wrote %s", outPath)
				return nil
			}
			if _, err := os.Stdout.Write(data); err != nil {
				return err
			}
			if len(data) > 0 && data[len(data)-1] != '\n' {
				fmt.Println()
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "name path segment")
	cmd.Flags().StringVar(&token, "token", "", "access token for private files")
	cmd.Flags().StringVar(&out, "out", "", "write body to this path")
	cmd.Flags().BoolVar(&download, "download", false, "request download disposition / extend retention")
	cmd.Flags().BoolVar(&meta, "meta", false, "fetch JSON metadata from API (?json=1)")
	return cmd
}
