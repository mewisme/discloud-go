package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info",
		Short: "Show public upload config",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			info, err := waitVal("Loading info…", c.GetInfo)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(info)
			}
			on := colorOn(os.Stdout)
			fmt.Printf("%s %s\n", dim(on, "bots:"), bold(on, fmt.Sprintf("%d", info.Bots)))
			fmt.Printf("%s %s\n", dim(on, "chunkSize:"), fmt.Sprintf("%d", info.ChunkSize))
			fmt.Printf("%s %s\n", dim(on, "workers:"), fmt.Sprintf("%d", info.Workers))
			return nil
		}),
	}
}

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "GET /healthz",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			s, err := waitVal("Checking health…", c.Health)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": s})
			}
			printSuccess("%s", s)
			return nil
		}),
	}
}

func newReadyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "GET /readyz",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			s, err := waitVal("Checking ready…", c.Ready)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": s})
			}
			printSuccess("%s", s)
			return nil
		}),
	}
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
