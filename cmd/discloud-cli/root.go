package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
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
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			printCLIHeader(cmd)
		},
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
		Short: "Show CLI version and build info",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			info := map[string]string{
				"name":     "discloud",
				"version":  version,
				"go":       runtime.Version(),
				"platform": runtime.GOOS + "/" + runtime.GOARCH,
			}
			if flagJSON {
				return writeJSON(info)
			}
			on := colorOn(os.Stdout)
			printSuccess("%s", bold(on, "DisCloud CLI"))
			fmt.Printf("%s %s\n", dim(on, "  version:"), green(on, info["version"]))
			fmt.Printf("%s %s\n", dim(on, "  go:"), dim(on, info["go"]))
			fmt.Printf("%s %s\n", dim(on, "  platform:"), cyan(on, info["platform"]))
			return nil
		}),
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
