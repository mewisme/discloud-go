package main

import (
	"fmt"
	"os"
	"strings"

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
	cmd.AddCommand(newConfigSetCmd(), newConfigUnsetCmd(), newConfigPathCmd())
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
	var setToken, clearToken bool
	cmd := &cobra.Command{
		Use:   "set [token]",
		Short: "Write base/origin/token to the user config.json",
		Args:  cobra.MaximumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				switch strings.ToLower(args[0]) {
				case "token":
					setToken = true
				default:
					return fmt.Errorf("unknown config field %q (want token, or use --base/--origin/--token)", args[0])
				}
			}

			cfg := client.DefaultConfig()
			baseSet := cmd.Flags().Changed("base")
			originSet := cmd.Flags().Changed("origin")
			tokenSet := setToken || clearToken

			promptedBaseOrigin := false
			promptedToken := false
			token := ""

			if !baseSet && !originSet && !tokenSet {
				if flagJSON {
					return fmt.Errorf("pass --base, --origin, and/or --token with --json")
				}
				var err error
				base, err = ui.PromptDefault("Base: ", cfg.BaseURL)
				if err != nil {
					return err
				}
				origin, err = ui.PromptDefault("Origin: ", cfg.Origin)
				if err != nil {
					return err
				}
				promptedBaseOrigin = true
			} else {
				if !baseSet {
					base = "" // keep file value via SaveConfigFile merge
				}
				if !originSet {
					origin = ""
				}
			}

			if clearToken {
				token = ""
			} else if setToken {
				if flagJSON {
					return fmt.Errorf("token prompts interactively; omit --json or set DISCLOUD_TOKEN in the environment")
				}
				var err error
				token, err = ui.ReadPasswordPrompt("Token: ")
				if err != nil {
					return err
				}
				token = strings.TrimSpace(token)
				if token == "" {
					return fmt.Errorf("token is required")
				}
				promptedToken = true
			}

			_, _, _, _, err := client.SaveConfigFileOpts(base, origin, token, tokenSet)
			if err != nil {
				return err
			}
			if flagJSON {
				cfg := client.DefaultConfig()
				out := map[string]string{
					"path":   client.ConfigFilePath(),
					"base":   cfg.BaseURL,
					"origin": cfg.Origin,
				}
				if tokenSet {
					out["token"] = maskAPIToken(cfg.Token)
				}
				return writeJSON(out)
			}
			if promptedBaseOrigin {
				ui.ClearLinesUp(os.Stderr, 2)
			}
			if promptedToken {
				ui.ClearLinesUp(os.Stderr, 1)
			}
			return printConfigTable(client.DefaultConfig())
		}),
	}
	cmd.Flags().StringVar(&base, "base", "", "API origin to save")
	cmd.Flags().StringVar(&origin, "origin", "", "WEB_ORIGIN to save")
	cmd.Flags().BoolVar(&setToken, "token", false, "Prompt for personal access token (value not passed on the CLI)")
	cmd.Flags().BoolVar(&clearToken, "clear-token", false, "Remove stored token (prefer: config unset token)")
	return cmd
}

func newConfigUnsetCmd() *cobra.Command {
	var clearBase, clearOrigin, clearToken bool
	cmd := &cobra.Command{
		Use:   "unset [base|origin|token]...",
		Short: "Clear fields from the user config.json",
		Args:  cobra.ArbitraryArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			for _, a := range args {
				switch strings.ToLower(strings.TrimSpace(a)) {
				case "base":
					clearBase = true
				case "origin":
					clearOrigin = true
				case "token":
					clearToken = true
				default:
					return fmt.Errorf("unknown config field %q (want base, origin, or token)", a)
				}
			}
			if cmd.Flags().Changed("base") {
				clearBase = true
			}
			if cmd.Flags().Changed("origin") {
				clearOrigin = true
			}
			if cmd.Flags().Changed("token") {
				clearToken = true
			}
			if !clearBase && !clearOrigin && !clearToken {
				return fmt.Errorf("usage: discloud config unset <base|origin|token>...")
			}
			if _, err := client.UnsetConfigFile(clearBase, clearOrigin, clearToken); err != nil {
				return err
			}
			if flagJSON {
				cfg := client.DefaultConfig()
				cleared := make([]string, 0, 3)
				if clearBase {
					cleared = append(cleared, "base")
				}
				if clearOrigin {
					cleared = append(cleared, "origin")
				}
				if clearToken {
					cleared = append(cleared, "token")
				}
				return writeJSON(map[string]any{
					"path":    client.ConfigFilePath(),
					"cleared": cleared,
					"base":    cfg.BaseURL,
					"origin":  cfg.Origin,
					"token":   maskAPIToken(cfg.Token),
				})
			}
			return printConfigTable(client.DefaultConfig())
		}),
	}
	cmd.Flags().BoolVar(&clearBase, "base", false, "Clear stored base")
	cmd.Flags().BoolVar(&clearOrigin, "origin", false, "Clear stored origin")
	cmd.Flags().BoolVar(&clearToken, "token", false, "Clear stored token")
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
			"token":   maskAPIToken(cfg.Token),
			"cookies": cfg.CookiePath,
			"file":    client.ConfigFilePath(),
		})
	}
	return printConfigTable(cfg)
}

// maskAPIToken shows dc_ + stars + last 5 chars (empty if unset).
func maskAPIToken(tok string) string {
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return ""
	}
	const keep = 5
	prefix := ""
	body := tok
	if strings.HasPrefix(tok, "dc_") {
		prefix = "dc_"
		body = tok[len(prefix):]
	}
	if len(body) <= keep {
		return prefix + strings.Repeat("*", len(body))
	}
	return prefix + strings.Repeat("*", len(body)-keep) + body[len(body)-keep:]
}

func printConfigTable(cfg client.Config) error {
	return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
		Title: "Config",
		Rows: [][]string{
			{"base", cfg.BaseURL},
			{"origin", cfg.Origin},
			{"token", maskAPIToken(cfg.Token)},
			{"cookies", cfg.CookiePath},
			{"file", client.ConfigFilePath()},
		},
	}})
}
