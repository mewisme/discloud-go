package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/ncruces/zenity"
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
			ui.JSON = flagJSON
			ui.PrintHeader(cmd)
		},
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.PersistentFlags().StringVar(&flagBase, "base", "", "API origin (env DISCLOUD_BASE)")
	cmd.PersistentFlags().StringVar(&flagOrigin, "origin", "", "must match server DISCLOUD_CORS for CSRF (env DISCLOUD_ORIGIN)")
	cmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit one JSON document on stdout")

	cmd.AddCommand(
		newAuthCmd(),
		newUploadCmd(),
		newChunksCmd(),
		newGetCmd(),
		newFilesCmd(),
		newAdminCmd(),
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
			return ui.PrintKVTable(os.Stdout, [][]string{
				{"version", info["version"]},
				{"go", info["go"]},
				{"platform", info["platform"]},
			})
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
		if err == nil {
			return nil
		}
		if isUserAbort(err) {
			// Silent exit 0 — avoid "exit status 1" from go run / shells.
			return nil
		}
		ui.PrintError(err)
		return err
	}
}

func isUserAbort(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, huh.ErrUserAborted) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, tea.ErrInterrupted) ||
		errors.Is(err, zenity.ErrCanceled) {
		return true
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return msg == "aborted" ||
		strings.Contains(msg, "user aborted") ||
		strings.Contains(msg, "dialog canceled") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "program was interrupted")
}
