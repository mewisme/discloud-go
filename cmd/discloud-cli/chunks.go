package main

import (
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/spf13/cobra"
)

func newChunksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chunks",
		Short: "Chunk store helpers",
	}
	cmd.AddCommand(newChunksCheckCmd(), newChunksPutCmd())
	return cmd
}

func newChunksCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [sha256]",
		Short: "Check whether a chunk exists",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			hash, err := ui.ResolveArg(ui.ArgOrEmpty(args, 0), "SHA-256: ")
			if err != nil {
				return err
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			ok, err := ui.WaitVal("Checking chunk…", func() (bool, error) {
				return c.ChunkExists(hash)
			})
			if err != nil {
				return err
			}
			return writeJSON(ChunkExistsResult{Exists: ok})
		}),
	}
}

func newChunksPutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "put [path]",
		Short: "Upload a raw chunk",
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
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var hash string
			var existed bool
			err = ui.WithSpinner("Uploading chunk…", func() error {
				var e error
				hash, existed, e = c.PutChunk(data)
				return e
			})
			if err != nil {
				return err
			}
			return writeJSON(ChunkPutResult{Hash: hash, Existed: existed})
		}),
	}
}
