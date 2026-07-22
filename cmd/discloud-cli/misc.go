package main

import (
	"fmt"
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

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or write client config",
		Args:  cobra.NoArgs,
		RunE:  runE(runConfigShow),
	}
	cmd.AddCommand(newConfigSetCmd(), newConfigPathCmd())
	return cmd
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config.json path",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path := client.ConfigFilePath()
			if flagJSON {
				return writeJSON(map[string]string{"path": path})
			}
			fmt.Println(path)
			return nil
		}),
	}
}

func newConfigSetCmd() *cobra.Command {
	var base, origin string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Write base/origin to the user config.json",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if base == "" && origin == "" {
				return fmt.Errorf("pass --base and/or --origin")
			}
			path, savedBase, savedOrigin, err := client.SaveConfigFile(base, origin)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{
					"path":   path,
					"base":   savedBase,
					"origin": savedOrigin,
				})
			}
			on := ui.ColorOn(os.Stdout)
			ui.PrintSuccess("Wrote %s", ui.Cyan(on, path))
			rows := make([][]string, 0, 2)
			if savedBase != "" {
				rows = append(rows, []string{"base", savedBase})
			}
			if savedOrigin != "" {
				rows = append(rows, []string{"origin", savedOrigin})
			}
			if len(rows) > 0 {
				return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{Title: "Config", Rows: rows}})
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&base, "base", "", "API origin to save")
	cmd.Flags().StringVar(&origin, "origin", "", "WEB_ORIGIN to save")
	return cmd
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	c, err := apiClient()
	if err != nil {
		return err
	}
	cfg := c.Config()
	if flagJSON {
		return writeJSON(map[string]string{
			"base":    cfg.BaseURL,
			"origin":  cfg.Origin,
			"cookies": cfg.CookiePath,
			"file":    client.ConfigFilePath(),
		})
	}
	return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
		Title: "Config",
		Rows: [][]string{
			{"base", cfg.BaseURL},
			{"origin", cfg.Origin},
			{"cookies", cfg.CookiePath},
			{"file", client.ConfigFilePath()},
		},
	}})
}
