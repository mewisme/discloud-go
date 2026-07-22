package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

var (
	flagBase   string
	flagOrigin string
	flagJSON   bool
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "discloud",
		Short:         "DisCloud API client",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.PersistentFlags().StringVar(&flagBase, "base", "", "API origin (env/`.env` DISCLOUD_BASE or API_URL)")
	cmd.PersistentFlags().StringVar(&flagOrigin, "origin", "", "WEB_ORIGIN for CSRF (env/`.env` DISCLOUD_ORIGIN or WEB_ORIGIN)")
	cmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit one JSON document on stdout")

	cmd.AddCommand(
		newAuthCmd(),
		newUploadCmd(),
		newUploadRawCmd(),
		newChunksCmd(),
		newGetCmd(),
		newFilesCmd(),
		newInfoCmd(),
		newHealthCmd(),
		newReadyCmd(),
		newConfigCmd(),
		newVersionCmd(),
	)
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}

func apiClient() (*client.Client, error) {
	cfg := client.DefaultConfig()
	if flagBase != "" {
		cfg.BaseURL = strings.TrimRight(flagBase, "/")
	}
	if flagOrigin != "" {
		cfg.Origin = strings.TrimRight(flagOrigin, "/")
	}
	return client.New(cfg)
}

func writeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func runE(fn func(cmd *cobra.Command, args []string) error) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		err := fn(cmd, args)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", errorMsg(fmt.Sprintf("discloud-cli: %v", err)))
		}
		return err
	}
}
