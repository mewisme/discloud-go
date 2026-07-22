package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "Check API liveness (/healthz)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			return runProbe("liveness", "/healthz", "Alive", func(c *client.Client) (string, error) {
				return waitVal("Checking health…", c.Health)
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
				return waitVal("Checking ready…", c.Ready)
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
	on := colorOn(os.Stdout)
	printSuccess("%s", bold(on, title))
	fmt.Printf("%s %s\n", dim(on, "  check:"), kind+" ("+path+")")
	fmt.Printf("%s %s\n", dim(on, "  base:"), cyan(on, base))
	fmt.Printf("%s %s\n", dim(on, "  status:"), green(on, status))
	return nil
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or write client config",
		Args:  cobra.NoArgs,
		RunE:  runE(runConfigShow),
	}
	cmd.AddCommand(newConfigShowCmd(), newConfigSetCmd(), newConfigPathCmd())
	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Show resolved client config",
		Args:  cobra.NoArgs,
		RunE:  runE(runConfigShow),
	}
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
			on := colorOn(os.Stdout)
			printSuccess("Wrote %s", cyan(on, path))
			if savedBase != "" {
				fmt.Printf("%s %s\n", dim(on, "base:"), cyan(on, savedBase))
			}
			if savedOrigin != "" {
				fmt.Printf("%s %s\n", dim(on, "origin:"), cyan(on, savedOrigin))
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
	on := colorOn(os.Stdout)
	fmt.Printf("%s %s\n", dim(on, "base:"), cyan(on, cfg.BaseURL))
	fmt.Printf("%s %s\n", dim(on, "origin:"), cyan(on, cfg.Origin))
	fmt.Printf("%s %s\n", dim(on, "cookies:"), dim(on, cfg.CookiePath))
	fmt.Printf("%s %s\n", dim(on, "file:"), dim(on, client.ConfigFilePath()))
	return nil
}
