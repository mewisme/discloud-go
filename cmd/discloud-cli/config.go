package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/transfer"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or write client config",
		Args:  cobra.NoArgs,
		RunE:  runE(runConfigShow),
	}
	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetCmd(),
		newConfigUnsetCmd(),
		newConfigPathCmd(),
	)
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Show client config (alias of config)",
		Args:  cobra.NoArgs,
		RunE:  runE(runConfigShow),
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config.yaml path",
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
		Short: "Write base/origin/token to the user config.yaml",
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

			if !baseSet && !originSet && !tokenSet {
				if flagJSON {
					return fmt.Errorf("pass --base, --origin, and/or --token with --json")
				}
				if !ui.IsTTY(os.Stdin) {
					return fmt.Errorf("pass --base, --origin, and/or --token (interactive prompts need a TTY)")
				}
				base = cfg.BaseURL
				origin = cfg.Origin
				if err := huh.NewInput().Title("Base").Value(&base).Run(); err != nil {
					return err
				}
				if err := huh.NewInput().Title("Origin").Value(&origin).Run(); err != nil {
					return err
				}
			} else {
				if !baseSet {
					base = ""
				}
				if !originSet {
					origin = ""
				}
			}

			token := ""
			if clearToken {
				token = ""
			} else if setToken {
				var err error
				token, err = promptPassword("Token")
				if err != nil {
					return err
				}
				token = strings.TrimSpace(token)
			}

			_, _, _, _, err := client.SaveConfigFileOpts(base, origin, token, tokenSet)
			if err != nil {
				return err
			}

			interactive := !flagJSON && ui.IsTTY(os.Stdin)
			if interactive {
				var wantSelf bool
				if err := huh.NewConfirm().
					Title("Configure self-upload?").
					Affirmative("Yes").
					Negative("No").
					Value(&wantSelf).
					Run(); err != nil {
					return err
				}
				if wantSelf {
					cur, err := client.LoadSelfUpload()
					if err != nil {
						return err
					}
					if err := promptAndSaveSelfUpload(cur); err != nil {
						return err
					}
				}
			}

			if flagJSON {
				cfg := client.DefaultConfig()
				out := map[string]any{
					"path":   client.ConfigFilePath(),
					"base":   cfg.BaseURL,
					"origin": cfg.Origin,
				}
				if tokenSet {
					out["token"] = maskAPIToken(cfg.Token)
				}
				return writeJSON(out)
			}
			return runConfigShow(cmd, nil)
		}),
	}
	cmd.Flags().StringVar(&base, "base", "", "API origin to save")
	cmd.Flags().StringVar(&origin, "origin", "", "DISCLOUD_CORS origin to save")
	cmd.Flags().BoolVar(&setToken, "token", false, "Prompt for personal access token (value not passed on the CLI)")
	cmd.Flags().BoolVar(&clearToken, "clear-token", false, "Remove stored token (prefer: config unset token)")
	cmd.AddCommand(newConfigSetSelfUploadCmd())
	return cmd
}

func newConfigUnsetCmd() *cobra.Command {
	var clearBase, clearOrigin, clearToken bool
	cmd := &cobra.Command{
		Use:   "unset [base|origin|token]...",
		Short: "Clear fields from the user config.yaml",
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
			return printClientConfig(client.DefaultConfig(), mustLoadSelfUpload())
		}),
	}
	cmd.Flags().BoolVar(&clearBase, "base", false, "Clear stored base")
	cmd.Flags().BoolVar(&clearOrigin, "origin", false, "Clear stored origin")
	cmd.Flags().BoolVar(&clearToken, "token", false, "Clear stored token")
	return cmd
}

func newConfigSetSelfUploadCmd() *cobra.Command {
	var (
		guildID, channelID string
		chunkMB            int
		appendTokens       bool
		clearTokens        bool
		clearAll           bool
		tokenFlags         []string
	)
	cmd := &cobra.Command{
		Use:   "self-upload",
		Short: "Configure Discord self-managed upload settings",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if clearAll {
				if err := client.SaveSelfUpload(nil); err != nil {
					return err
				}
				if flagJSON {
					return writeJSON(map[string]string{"cleared": "selfUpload", "path": client.ConfigFilePath()})
				}
				ui.PrintSuccess("Cleared self-upload config")
				return printClientConfig(client.DefaultConfig(), client.SelfUploadConfig{})
			}
			cur, err := client.LoadSelfUpload()
			if err != nil {
				return err
			}
			interactive := !flagJSON && ui.IsTTY(os.Stdin)

			if guildID == "" {
				guildID = cur.GuildID
			}
			if channelID == "" {
				channelID = cur.ChannelID
			}
			if !cmd.Flags().Changed("chunk-mb") && cur.ChunkSize > 0 {
				chunkMB = transfer.BytesToMB(cur.ChunkSize)
			}

			if guildID == "" || channelID == "" || (len(tokenFlags) == 0 && !appendTokens && !clearTokens && len(cur.BotTokens) == 0) {
				if !interactive {
					return fmt.Errorf("self-upload requires --guild-id, --channel-id, and bot tokens (or run in a TTY)")
				}
				if guildID != "" {
					cur.GuildID = guildID
				}
				if channelID != "" {
					cur.ChannelID = channelID
				}
				if chunkMB > 0 {
					cur.ChunkSize = transfer.MBToBytes(chunkMB)
				}
				return promptAndSaveSelfUpload(cur)
			}

			su := cur
			su.GuildID = strings.TrimSpace(guildID)
			su.ChannelID = strings.TrimSpace(channelID)
			if clearTokens {
				su.BotTokens = nil
			}
			if len(tokenFlags) > 0 {
				if appendTokens {
					su.BotTokens = append(su.BotTokens, tokenFlags...)
				} else {
					su.BotTokens = tokenFlags
				}
			}
			su.BotTokens = client.NormalizeBotTokens(su.BotTokens)
			if chunkMB > 0 {
				su.ChunkSize = transfer.MBToBytes(chunkMB)
			}
			if su.ChunkSize <= 0 {
				su.ChunkSize = transfer.MaxChunkForTier(0)
			}
			if err := client.SaveSelfUpload(&su); err != nil {
				return err
			}
			if n := len(su.BotTokens); n > 0 && n < 5 {
				fmt.Fprintln(os.Stderr, "For better upload performance, configuring at least 5 Discord bot tokens is recommended.")
			}
			if flagJSON {
				return writeJSON(map[string]any{
					"path":          client.ConfigFilePath(),
					"guildId":       su.GuildID,
					"channelId":     su.ChannelID,
					"botTokenCount": len(su.BotTokens),
					"premiumTier":   su.PremiumTier,
					"chunkSize":     su.ChunkSize,
				})
			}
			return printClientConfig(client.DefaultConfig(), su)
		}),
	}
	cmd.Flags().StringVar(&guildID, "guild-id", "", "Discord guild ID")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "Discord channel ID")
	cmd.Flags().IntVar(&chunkMB, "chunk-mb", 0, "Preferred chunk size in MB (stored as bytes)")
	cmd.Flags().StringArrayVar(&tokenFlags, "bot-token", nil, "Bot token (repeatable; prefer prompts to avoid shell history)")
	cmd.Flags().BoolVar(&appendTokens, "append-tokens", false, "Append tokens instead of replacing")
	cmd.Flags().BoolVar(&clearTokens, "clear-tokens", false, "Clear all bot tokens")
	cmd.Flags().BoolVar(&clearAll, "clear", false, "Clear entire self-upload config")
	return cmd
}

func promptAndSaveSelfUpload(cur client.SelfUploadConfig) error {
	guildID := cur.GuildID
	channelID := cur.ChannelID
	chunkMB := 0
	if cur.ChunkSize > 0 {
		chunkMB = transfer.BytesToMB(cur.ChunkSize)
	}

	if err := huh.NewInput().Title("Discord guild ID").Value(&guildID).Run(); err != nil {
		return err
	}
	if err := huh.NewInput().Title("Discord channel ID").Value(&channelID).Run(); err != nil {
		return err
	}
	tokens := cur.BotTokens
	if len(tokens) == 0 {
		var err error
		tokens, err = promptBotTokens()
		if err != nil {
			return err
		}
	} else {
		ui.PrintInfo("Bot tokens: %d already configured (kept). Re-run with: discloud config set self-upload --clear-tokens", len(tokens))
	}
	if chunkMB <= 0 {
		chunkMB = 10
	}
	var s string
	if err := huh.NewInput().Title("Preferred chunk size in MB").Value(&s).Placeholder(strconv.Itoa(chunkMB)).Run(); err != nil {
		return err
	}
	if strings.TrimSpace(s) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid chunk size MB")
		}
		chunkMB = n
	}

	su := cur
	su.GuildID = strings.TrimSpace(guildID)
	su.ChannelID = strings.TrimSpace(channelID)
	su.BotTokens = client.NormalizeBotTokens(tokens)
	su.ChunkSize = transfer.MBToBytes(chunkMB)
	if su.ChunkSize <= 0 {
		su.ChunkSize = transfer.MaxChunkForTier(0)
	}
	if err := client.SaveSelfUpload(&su); err != nil {
		return err
	}
	if n := len(su.BotTokens); n > 0 && n < 5 {
		fmt.Fprintln(os.Stderr, "For better upload performance, configuring at least 5 Discord bot tokens is recommended.")
	}
	if flagJSON {
		return writeJSON(map[string]any{
			"path":          client.ConfigFilePath(),
			"guildId":       su.GuildID,
			"channelId":     su.ChannelID,
			"botTokenCount": len(su.BotTokens),
			"premiumTier":   su.PremiumTier,
			"chunkSize":     su.ChunkSize,
		})
	}
	return printClientConfig(client.DefaultConfig(), su)
}

// promptBotTokens collects tokens. Comma-separated values count as multiple tokens
// and advance the #N label. Two consecutive empty entries finish the prompt.
func promptBotTokens() ([]string, error) {
	var tokens []string
	emptyStreak := 0
	for {
		var tok string
		n := len(tokens) + 1
		err := huh.NewInput().
			Title(fmt.Sprintf("Discord bot token #%d", n)).
			Description("Paste one token, or several separated by commas. Empty twice to continue.").
			EchoMode(huh.EchoModePassword).
			Value(&tok).
			Run()
		if err != nil {
			return nil, err
		}
		parts := splitCommaFields(tok)
		if len(parts) == 0 {
			emptyStreak++
			if emptyStreak >= 2 {
				break
			}
			continue
		}
		emptyStreak = 0
		tokens = append(tokens, parts...)
	}
	tokens = client.NormalizeBotTokens(tokens)
	if len(tokens) == 0 {
		return nil, fmt.Errorf("at least one bot token is required")
	}
	return tokens, nil
}

// splitCommaFields splits on commas, trims, and drops empties.
func splitCommaFields(s string) []string {
	if !strings.Contains(s, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil
		}
		return []string{s}
	}
	raw := strings.Split(s, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	_ = client.EnsureConfigMigrated()
	c, err := apiClient()
	if err != nil {
		return err
	}
	cfg := c.Config()
	su, _ := client.LoadSelfUpload()
	if flagJSON {
		return writeJSON(map[string]any{
			"base":    cfg.BaseURL,
			"origin":  cfg.Origin,
			"token":   maskAPIToken(cfg.Token),
			"cookies": cfg.CookiePath,
			"file":    client.ConfigFilePath(),
			"selfUpload": map[string]any{
				"configured":    su.Configured(),
				"botTokenCount": len(su.BotTokens),
				"guildId":       su.GuildID,
				"channelId":     su.ChannelID,
				"premiumTier":   su.PremiumTier,
				"chunkSize":     su.ChunkSize,
			},
		})
	}
	return printClientConfig(cfg, su)
}

func mustLoadSelfUpload() client.SelfUploadConfig {
	su, _ := client.LoadSelfUpload()
	return su
}

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

func configKVBlock(cfg client.Config) ui.KVBlock {
	return ui.KVBlock{
		Title: "Config",
		Rows: [][]string{
			{"base", cfg.BaseURL},
			{"origin", cfg.Origin},
			{"token", maskAPIToken(cfg.Token)},
			{"cookies", cfg.CookiePath},
			{"file", client.ConfigFilePath()},
		},
	}
}

func selfUploadKVBlock(su client.SelfUploadConfig) ui.KVBlock {
	chunk := "-"
	if su.ChunkSize > 0 {
		chunk = transfer.FormatChunkMB(su.ChunkSize)
	}
	return ui.KVBlock{
		Title: "Self-upload",
		Rows: [][]string{
			{"Bot tokens", fmt.Sprintf("%d configured", len(su.BotTokens))},
			{"Guild ID", orDash(su.GuildID)},
			{"Channel ID", orDash(su.ChannelID)},
			{"Cached premium tier", strconv.Itoa(su.PremiumTier)},
			{"Chunk size", chunk},
		},
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

// printClientConfig renders Config + Self-upload as one admin-overview-style table.
func printClientConfig(cfg client.Config, su client.SelfUploadConfig) error {
	blocks := []ui.KVBlock{configKVBlock(cfg)}
	if su.Configured() || len(su.BotTokens) > 0 || su.ChannelID != "" || su.GuildID != "" {
		blocks = append(blocks, selfUploadKVBlock(su))
	}
	return ui.PrintKVBlocks(os.Stdout, blocks)
}
