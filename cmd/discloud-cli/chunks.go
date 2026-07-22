package main

import (
	"os"

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
		Use:   "check <sha256>",
		Short: "Check whether a chunk exists",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			ok, err := waitVal("Checking chunk…", func() (bool, error) {
				return c.ChunkExists(args[0])
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
		Use:   "put <path>",
		Short: "Upload a raw chunk",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var hash string
			var existed bool
			err = withSpinner("Uploading chunk…", func() error {
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
