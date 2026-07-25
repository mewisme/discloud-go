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
				if err := runHuh(huh.NewInput().Title("Base").Value(&base)); err != nil {
					return err
				}
				if err := runHuh(huh.NewInput().Title("Origin").Value(&origin)); err != nil {
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
				if err := runHuh(huh.NewConfirm().
					Title("Configure self-upload?").
					Affirmative("Yes").
					Negative("No").
					Value(&wantSelf)); err != nil {
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
			return printClientConfigAfterSet(client.DefaultConfig(), mustLoadSelfUpload())
		}),
	}
	cmd.Flags().StringVar(&base, "base", "", "API origin to save")
	cmd.Flags().StringVar(&origin, "origin", "", "DISCLOUD_CORS origin to save")
	cmd.Flags().BoolVar(&setToken, "token", false, "Prompt for personal access token (value not passed on the CLI)")
	cmd.Flags().BoolVar(&clearToken, "clear-token", false, "Remove stored token (prefer: config unset token)")
	cmd.AddCommand(newConfigSetSelfUploadCmd(), newConfigSetCustomizeCmd())
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
			return printClientConfigAfterSet(client.DefaultConfig(), mustLoadSelfUpload())
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
			return printClientConfigAfterSet(client.DefaultConfig(), su)
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

	if err := runHuh(huh.NewInput().Title("Discord guild ID").Value(&guildID)); err != nil {
		return err
	}
	if err := runHuh(huh.NewInput().Title("Discord channel ID").Value(&channelID)); err != nil {
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
	if err := runHuh(huh.NewInput().Title("Preferred chunk size in MB").Value(&s).Placeholder(strconv.Itoa(chunkMB))); err != nil {
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
	return printClientConfigAfterSet(client.DefaultConfig(), su)
}

// promptBotTokens collects tokens. Comma-separated values count as multiple tokens
// and advance the #N label. Two consecutive empty entries finish the prompt.
func promptBotTokens() ([]string, error) {
	var tokens []string
	emptyStreak := 0
	for {
		var tok string
		n := len(tokens) + 1
		err := runHuh(huh.NewInput().
			Title(fmt.Sprintf("Discord bot token #%d", n)).
			Description("Paste one token, or several separated by commas. Empty twice to continue.").
			EchoMode(huh.EchoModePassword).
			Value(&tok))
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

func newConfigSetCustomizeCmd() *cobra.Command {
	var uploadColors, downloadColors, promptTheme, align string
	var resetScope string
	var uploadLongHue, downloadLongHue bool
	cmd := &cobra.Command{
		Use:   "customize",
		Short: "Configure progress bar colors, prompt theme, and align",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			cur, err := client.LoadCustomize()
			if err != nil {
				return err
			}
			upFlag := cmd.Flags().Changed("upload-colors")
			downFlag := cmd.Flags().Changed("download-colors")
			upLongFlag := cmd.Flags().Changed("upload-long-hue")
			downLongFlag := cmd.Flags().Changed("download-long-hue")
			themeFlag := cmd.Flags().Changed("prompt-theme")
			alignFlag := cmd.Flags().Changed("align")
			resetFlag := cmd.Flags().Changed("reset")

			if resetFlag {
				if err := applyCustomizeReset(&cur, resetScope); err != nil {
					return err
				}
				if err := client.SaveCustomize(&cur); err != nil {
					return err
				}
				if flagJSON {
					return writeCustomizeJSON(cur)
				}
				return printClientConfigAfterSet(client.DefaultConfig(), mustLoadSelfUpload())
			}

			if upFlag || downFlag || upLongFlag || downLongFlag || themeFlag || alignFlag {
				if themeFlag {
					theme, err := client.NormalizePromptTheme(promptTheme)
					if err != nil {
						return err
					}
					cur.PromptTheme = theme
				}
				if alignFlag {
					a, err := client.NormalizeAlign(align)
					if err != nil {
						return err
					}
					cur.Align = a
				}
				if upFlag {
					colors, err := parseColorFlag(uploadColors)
					if err != nil {
						return fmt.Errorf("upload-colors: %w", err)
					}
					longHue := uploadLongHue
					if !upLongFlag {
						_, longHue = cur.ProgressBarSettings("upload")
					}
					cur.Upload = &client.TransferUIConfig{
						ProgressBar: client.ProgressBarConfig{Colors: colors, LongHue: longHue},
					}
				} else if upLongFlag {
					colors := cur.ProgressColors("upload")
					cur.Upload = &client.TransferUIConfig{
						ProgressBar: client.ProgressBarConfig{Colors: colors, LongHue: uploadLongHue},
					}
				}
				if downFlag {
					colors, err := parseColorFlag(downloadColors)
					if err != nil {
						return fmt.Errorf("download-colors: %w", err)
					}
					longHue := downloadLongHue
					if !downLongFlag {
						_, longHue = cur.ProgressBarSettings("download")
					}
					cur.Download = &client.TransferUIConfig{
						ProgressBar: client.ProgressBarConfig{Colors: colors, LongHue: longHue},
					}
				} else if downLongFlag {
					colors := cur.ProgressColors("download")
					cur.Download = &client.TransferUIConfig{
						ProgressBar: client.ProgressBarConfig{Colors: colors, LongHue: downloadLongHue},
					}
				}
				if err := client.SaveCustomize(&cur); err != nil {
					return err
				}
				if flagJSON {
					return writeCustomizeJSON(cur)
				}
				return printClientConfigAfterSet(client.DefaultConfig(), mustLoadSelfUpload())
			}

			if flagJSON {
				return fmt.Errorf("pass --upload-colors, --download-colors, --upload-long-hue, --download-long-hue, --prompt-theme, --align, and/or --reset with --json")
			}
			if !ui.IsTTY(os.Stdin) {
				return fmt.Errorf("pass flags or run in a TTY for interactive customize")
			}

			if err := promptAndSaveCustomize(cur); err != nil {
				return err
			}
			return printClientConfigAfterSet(client.DefaultConfig(), mustLoadSelfUpload())
		}),
	}
	cmd.Flags().StringVar(&uploadColors, "upload-colors", "", "Comma-separated hex colors for upload progress bar")
	cmd.Flags().StringVar(&downloadColors, "download-colors", "", "Comma-separated hex colors for download progress bar")
	cmd.Flags().BoolVar(&uploadLongHue, "upload-long-hue", false, "HSV long-arc blend between upload color stops")
	cmd.Flags().BoolVar(&downloadLongHue, "download-long-hue", false, "HSV long-arc blend between download color stops")
	cmd.Flags().StringVar(&promptTheme, "prompt-theme", "", "Huh prompt theme: base16, charm, dracula, catppuccin, default")
	cmd.Flags().StringVar(&align, "align", "", "UI content align: left or center (default center)")
	cmd.Flags().StringVar(&resetScope, "reset", "", "Reset to default gradient: upload, download, or both")
	if f := cmd.Flags().Lookup("reset"); f != nil {
		f.NoOptDefVal = "both"
	}
	return cmd
}

func applyCustomizeReset(cur *client.CustomizeConfig, scope string) error {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case "", "both", "all":
		cur.Upload = nil
		cur.Download = nil
	case "upload":
		cur.Upload = nil
	case "download":
		cur.Download = nil
	default:
		return fmt.Errorf("reset: want upload, download, or both (got %q)", scope)
	}
	return nil
}

// promptAndSaveCustomize walks customize → upload|download|prompt theme|align → picker.
func promptAndSaveCustomize(cur client.CustomizeConfig) error {
	var side string
	if err := runHuh(huh.NewSelect[string]().
		Title("customize").
		Description("What to configure?").
		Options(
			huh.NewOption("upload progress bar colors", "upload"),
			huh.NewOption("download progress bar colors", "download"),
			huh.NewOption("both progress bars (same colors)", "both"),
			huh.NewOption("prompt theme (huh forms)", "theme"),
			huh.NewOption("align (banner + tables + prompts)", "align"),
			huh.NewOption("reset progress colors to default", "reset"),
		).
		Value(&side)); err != nil {
		return err
	}

	if side == "theme" {
		theme, err := promptPromptTheme(cur.PromptTheme)
		if err != nil {
			return err
		}
		cur.PromptTheme = theme
		return client.SaveCustomize(&cur)
	}

	if side == "align" {
		pick := cur.AlignOrDefault()
		if err := runHuh(huh.NewSelect[string]().
			Title("customize.align").
			Description("UI content alignment (tables, prompts, messages). Default: center.").
			Options(
				huh.NewOption("center (banner + content)", "center"),
				huh.NewOption("left (banner + content)", "left"),
			).
			Value(&pick)); err != nil {
			return err
		}
		cur.Align = pick
		return client.SaveCustomize(&cur)
	}

	if side == "reset" {
		var scope string
		if err := runHuh(huh.NewSelect[string]().
			Title("customize.reset").
			Description("Clear stored colors (built-in gradient used until you set new ones).").
			Options(
				huh.NewOption("upload", "upload"),
				huh.NewOption("download", "download"),
				huh.NewOption("both", "both"),
			).
			Value(&scope)); err != nil {
			return err
		}
		if err := applyCustomizeReset(&cur, scope); err != nil {
			return err
		}
		if err := client.SaveCustomize(&cur); err != nil {
			return err
		}
		return nil
	}

	seedSide := side
	if side == "both" {
		seedSide = "upload"
	}
	ui.PrintInfo("customize.%s.progressbar.colors", side)
	curColors, curLong := cur.ProgressBarSettings(seedSide)
	if side == "both" && len(curColors) == 0 {
		curColors, curLong = cur.ProgressBarSettings("download")
	}
	colors, longHue, err := promptProgressColors(side, curColors, curLong)
	if err != nil {
		return err
	}
	block := client.ProgressBarConfig{Colors: colors, LongHue: longHue}
	switch side {
	case "upload":
		cur.Upload = &client.TransferUIConfig{ProgressBar: block}
	case "download":
		cur.Download = &client.TransferUIConfig{ProgressBar: block}
	case "both":
		cur.Upload = &client.TransferUIConfig{ProgressBar: block}
		cur.Download = &client.TransferUIConfig{ProgressBar: client.ProgressBarConfig{
			Colors:  append([]string(nil), colors...),
			LongHue: longHue,
		}}
	}

	return client.SaveCustomize(&cur)
}

func parseColorFlag(s string) ([]string, error) {
	parts := splitCommaFields(s)
	if len(parts) == 0 {
		return nil, fmt.Errorf("at least one color required")
	}
	return client.NormalizeHexColors(parts)
}

func promptProgressColors(label string, current []string, curLongHue bool) (colors []string, longHue bool, err error) {
	title := label
	if label != "" {
		title = strings.ToUpper(label[:1]) + label[1:]
	}
	show := current
	if len(show) == 0 {
		show = ui.DefaultProgressColors()
	}
	ui.PrintProgressPreviewOpts(os.Stderr, title+" (current)", show, curLongHue)

	defSteps := len(show)
	if defSteps < minGradSteps {
		defSteps = len(ui.DefaultProgressColors())
	}
	var raw string
	if err := runHuh(huh.NewInput().
		Title("Gradient steps").
		Description(fmt.Sprintf("How many color stops? (%d–%d)", minGradSteps, maxGradSteps)).
		Placeholder(strconv.Itoa(defSteps)).
		Value(&raw)); err != nil {
		return nil, false, err
	}
	n := defSteps
	if s := strings.TrimSpace(raw); s != "" {
		parsed, err := strconv.Atoi(s)
		if err != nil || parsed < minGradSteps || parsed > maxGradSteps {
			return nil, false, fmt.Errorf("gradient steps must be %d–%d", minGradSteps, maxGradSteps)
		}
		n = parsed
	}

	seed := seedGradientSteps(n, current)
	colors, longHue, canceled, err := promptHexColors(title, seed, curLongHue)
	if err != nil {
		return nil, false, err
	}
	if canceled {
		return nil, false, fmt.Errorf("%s colors canceled", label)
	}
	if len(colors) == 0 {
		return nil, false, fmt.Errorf("%s requires at least one color", label)
	}
	return colors, longHue, nil
}

func writeCustomizeJSON(c client.CustomizeConfig) error {
	up, upLH := c.ProgressBarSettings("upload")
	down, downLH := c.ProgressBarSettings("download")
	return writeJSON(map[string]any{
		"path": client.ConfigFilePath(),
		"customize": map[string]any{
			"promptTheme": c.PromptThemeOrDefault(),
			"align":       c.AlignOrDefault(),
			"upload": map[string]any{
				"progressbar": map[string]any{"colors": up, "longHue": upLH},
			},
			"download": map[string]any{
				"progressbar": map[string]any{"colors": down, "longHue": downLH},
			},
		},
	})
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	_ = client.EnsureConfigMigrated()
	c, err := apiClient()
	if err != nil {
		return err
	}
	cfg := c.Config()
	su, _ := client.LoadSelfUpload()
	cust, _ := client.LoadCustomize()
	upCols, upLH := cust.ProgressBarSettings("upload")
	downCols, downLH := cust.ProgressBarSettings("download")
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
			"customize": map[string]any{
				"promptTheme": cust.PromptThemeOrDefault(),
				"align":       cust.AlignOrDefault(),
				"upload": map[string]any{
					"progressbar": map[string]any{"colors": upCols, "longHue": upLH},
				},
				"download": map[string]any{
					"progressbar": map[string]any{"colors": downCols, "longHue": downLH},
				},
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

// printClientConfigAfterSet shows the config table (clear + banner + centered).
func printClientConfigAfterSet(cfg client.Config, su client.SelfUploadConfig) error {
	return printClientConfig(cfg, su)
}

// printClientConfig renders Config + Self-upload + Customize as KV blocks.
func printClientConfig(cfg client.Config, su client.SelfUploadConfig) error {
	ui.SyncContentAlign()
	blocks := []ui.KVBlock{configKVBlock(cfg)}
	if su.Configured() || len(su.BotTokens) > 0 || su.ChannelID != "" || su.GuildID != "" {
		blocks = append(blocks, selfUploadKVBlock(su))
	}
	cust, _ := client.LoadCustomize()
	blocks = append(blocks, customizeKVBlock(cust, 0))
	barW := ui.KVBlocksValueWidth(os.Stdout, blocks)
	blocks[len(blocks)-1] = customizeKVBlock(cust, barW)
	return ui.PrintKVBlocks(os.Stdout, blocks)
}

func customizeKVBlock(c client.CustomizeConfig, barWidth int) ui.KVBlock {
	up, upLH := c.ProgressBarSettings("upload")
	down, downLH := c.ProgressBarSettings("download")
	upPrev, downPrev := "", ""
	if barWidth > 0 {
		upPrev = ui.ProgressBarPreview(up, upLH, barWidth)
		downPrev = ui.ProgressBarPreview(down, downLH, barWidth)
	}
	return ui.KVBlock{
		Title: "Customize",
		Rows: [][]string{
			{"prompt theme", promptThemeLabel(c.PromptThemeOrDefault())},
			{"align", c.AlignOrDefault()},
			{"upload progressbar", upPrev},
			{"upload longHue", formatBool(upLH)},
			{"download progressbar", downPrev},
			{"download longHue", formatBool(downLH)},
		},
	}
}

func formatBool(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
