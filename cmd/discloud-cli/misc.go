package main

import (
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check API liveness (/healthz)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			return runProbe("liveness", "/healthz", "Health", func(c *client.Client) (string, error) {
				return ui.WaitVal("Checking health…", c.Health)
			})
		}),
	}
}

func newReadyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "Check API readiness (/readyz)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			return runProbe("readiness", "/readyz", "Ready", func(c *client.Client) (string, error) {
				return ui.WaitVal("Checking ready…", c.Ready)
			})
		}),
	}
}

// runProbe hits a status endpoint and prints a labeled human summary (or JSON).
func runProbe(kind, path, title string, call func(*client.Client) (string, error)) error {
	c, err := apiClient()
	if err != nil {
		return err
	}
	status, err := call(c)
	if err != nil {
		return err
	}
	base := c.Config().BaseURL
	if flagJSON {
		return writeJSON(map[string]string{
			"check":    kind,
			"endpoint": path,
			"base":     base,
			"status":   status,
		})
	}
	return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
		Title: title,
		Rows: [][]string{
			{"check", kind + " (" + path + ")"},
			{"base", base},
			{"status", status},
		},
	}})
}
