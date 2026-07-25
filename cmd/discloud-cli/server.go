package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/localserver"
	"github.com/mewisme/discloud-go/internal/serverconfig"
	"github.com/mewisme/discloud-go/internal/userdir"
	"github.com/spf13/cobra"
)

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Install and manage a local DisCloud server",
	}
	cmd.AddCommand(
		newServerInstallCmd(),
		newServerUpdateCmd(),
		newServerUninstallCmd(),
		newServerStartCmd(),
		newServerStopCmd(),
		newServerRestartCmd(),
		newServerStatusCmd(),
		newServerLogsCmd(),
		newServerVersionCmd(),
		newServerConfigCmd(),
	)
	return cmd
}

func newServerInstallCmd() *cobra.Command {
	var ver string
	var yes bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Download and install a local DisCloud server (does not start it)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			opts := localserver.InstallOptions{Version: ver, Yes: yes}
			if !flagJSON && ui.IsTTY(os.Stdin) {
				if err := promptServerBasics(&opts); err != nil {
					return err
				}
			}
			res, err := ui.WaitVal("Installing local server…", func() (localserver.CLIInstallResult, error) {
				return localserver.InstallCLI(cmd.Context(), opts)
			})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(res)
			}
			ui.PrintSuccess("Installed DisCloud server %s", ui.Bold(ui.ColorOn(os.Stdout), res.Version))
			ui.PrintInfo("Run `discloud server start` to start the local server.")
			return nil
		}),
	}
	cmd.Flags().StringVar(&ver, "version", "", "release tag (default: latest stable)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmations")
	return cmd
}

func newServerUpdateCmd() *cobra.Command {
	var ver string
	var yes bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the local DisCloud server binary",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			res, err := ui.WaitVal("Updating local server…", func() (localserver.CLIInstallResult, error) {
				return localserver.UpdateCLI(cmd.Context(), localserver.InstallOptions{Version: ver, Yes: yes})
			})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(res)
			}
			ui.PrintSuccess("Updated DisCloud server to %s", ui.Bold(ui.ColorOn(os.Stdout), res.Version))
			if res.Restarted {
				ui.PrintInfo("Server was running and has been restarted.")
			} else {
				ui.PrintInfo("Server left stopped (it was not running before the update).")
			}
			return nil
		}),
	}
	cmd.Flags().StringVar(&ver, "version", "", "release tag (default: latest stable)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmations")
	return cmd
}

func newServerUninstallCmd() *cobra.Command {
	var purge, yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the local server binary (preserves config/data unless --purge)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if purge && !yes {
				if err := promptConfirm("Permanently delete server config, database, logs, and secrets?"); err != nil {
					return err
				}
			} else if !yes && !flagJSON {
				if err := promptConfirm("Uninstall local DisCloud server binaries?"); err != nil {
					return err
				}
			} else if !yes && flagJSON {
				return fmt.Errorf("refusing without -y")
			}
			res, err := localserver.UninstallCLI(cmd.Context(), localserver.UninstallOptions{Purge: purge, Yes: yes})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(res)
			}
			ui.PrintSuccess("Local server uninstalled")
			for _, line := range res.Messages {
				ui.PrintInfo("%s", line)
			}
			return nil
		}),
	}
	cmd.Flags().BoolVar(&purge, "purge", false, "also delete config, database, logs, and secrets")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmations")
	return cmd
}

func newServerStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the local DisCloud server in the background",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			st, err := ui.WaitVal("Starting local server…", func() (localserver.CLIStatus, error) {
				return localserver.StartDefault(cmd.Context())
			})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(st)
			}
			ui.PrintSuccess("Server started (pid %d)", st.PID)
			return nil
		}),
	}
}

func newServerStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the local DisCloud server",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			_, err := ui.WaitVal("Stopping local server…", func() (struct{}, error) {
				return struct{}{}, localserver.StopDefault(cmd.Context())
			})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "stopped"})
			}
			ui.PrintSuccess("Server stopped")
			return nil
		}),
	}
}

func newServerRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the local DisCloud server",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			st, err := ui.WaitVal("Restarting local server…", func() (localserver.CLIStatus, error) {
				return localserver.RestartDefault(cmd.Context())
			})
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(st)
			}
			ui.PrintSuccess("Server restarted (pid %d)", st.PID)
			return nil
		}),
	}
}

func newServerStatusCmd() *cobra.Command {
	var showSecret bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local server installation and process status",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			st, err := localserver.GetStatusDefault(cmd.Context())
			if err != nil {
				return err
			}
			if !showSecret {
				st = st.Masked()
			}
			if flagJSON {
				return writeJSON(st)
			}
			return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
				Title: "Local server",
				Rows: [][]string{
					{"installed", strconv.FormatBool(st.Installed)},
					{"version", st.Version},
					{"binary", st.Binary},
					{"config", st.ConfigPath},
					{"env", st.EnvPath},
					{"dataDir", st.DataDir},
					{"database", st.DatabasePath},
					{"log", st.LogPath},
					{"process", st.ProcessState},
					{"pid", formatPID(st.PID)},
					{"health", st.HealthState},
					{"ready", st.ReadyState},
					{"apiUrl", st.APIURL},
					{"store", st.StoreBackend},
					{"cache", st.CacheBackend},
				},
			}})
		}),
	}
	cmd.Flags().BoolVar(&showSecret, "show-secret", false, "show unmasked secrets in output")
	return cmd
}

func newServerLogsCmd() *cobra.Command {
	var tail int
	follow := true
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Stream local server logs (Ctrl+C to stop)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path := localserver.LogPathDefault()
			if flagJSON {
				// Machine output is always a one-shot tail of raw lines.
				lines, err := localserver.TailLogDefault(tail)
				if err != nil {
					return err
				}
				return writeJSON(map[string]any{"path": path, "lines": lines})
			}
			fmt.Fprintln(os.Stderr, path)
			ctx, stop := ui.ContextWithInterrupt(context.Background())
			defer stop()
			err := localserver.PrintLogsDefault(ctx, logDisplay{w: os.Stdout}, tail, follow)
			if err != nil && errors.Is(err, context.Canceled) {
				return context.Canceled
			}
			return err
		}),
	}
	cmd.Flags().IntVar(&tail, "tail", 100, "number of recent lines to show first")
	cmd.Flags().BoolVarP(&follow, "follow", "f", true, "stream new lines until Ctrl+C (default true; --follow=false for one-shot)")
	return cmd
}

func newServerVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show installed local server version",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			info, err := localserver.VersionInfoDefault()
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(info)
			}
			return ui.PrintKVTable(os.Stdout, [][]string{
				{"installed", strconv.FormatBool(info.Installed)},
				{"version", info.Version},
				{"binary", info.Binary},
			})
		}),
	}
}

func newServerConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or edit local server configuration",
		Args:  cobra.NoArgs,
		RunE:  runE(runServerConfigShow),
	}
	cmd.AddCommand(
		newServerConfigShowCmd(),
		newServerConfigGetCmd(),
		newServerConfigSetCmd(),
		newServerConfigUnsetCmd(),
		newServerConfigPathCmd(),
	)
	return cmd
}

func newServerConfigShowCmd() *cobra.Command {
	var showSecret bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show server.yaml (secrets masked by default)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			return runServerConfigShowWith(showSecret)
		}),
	}
	cmd.Flags().BoolVar(&showSecret, "show-secret", false, "show unmasked secrets")
	return cmd
}

func runServerConfigShow(cmd *cobra.Command, args []string) error {
	return runServerConfigShowWith(false)
}

func runServerConfigShowWith(showSecret bool) error {
	f, err := serverconfig.Load(userdir.ServerInstallDir())
	if err != nil {
		return err
	}
	out := f
	if !showSecret {
		out = serverconfig.MaskSecrets(f)
	}
	if flagJSON {
		return writeJSON(out)
	}
	return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
		Title: "Server config",
		Rows: [][]string{
			{"path", serverconfig.Path(userdir.ServerInstallDir())},
			{"host", out.Host},
			{"port", out.Port},
			{"botTokens", strings.Join(out.BotTokens, ", ")},
			{"channelId", out.ChannelID},
			{"cors", out.CORS},
			{"apiUrl", out.APIURL},
			{"logLevel", out.LogLevel},
			{"databaseUrl", out.DatabaseURL},
			{"valkeyUrl", out.ValkeyURL},
		},
	}})
}

func newServerConfigGetCmd() *cobra.Command {
	var showSecret bool
	cmd := &cobra.Command{
		Use:   "get <key>",
		Short: "Get one server config key",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			f, err := serverconfig.Load(userdir.ServerInstallDir())
			if err != nil {
				return err
			}
			v, err := serverconfig.Get(f, args[0])
			if err != nil {
				return err
			}
			if !showSecret && (strings.Contains(strings.ToLower(args[0]), "bot") || strings.Contains(strings.ToLower(args[0]), "token")) {
				mf := serverconfig.MaskSecrets(f)
				v, _ = serverconfig.Get(mf, args[0])
			}
			if flagJSON {
				return writeJSON(map[string]string{"key": args[0], "value": v})
			}
			fmt.Println(v)
			return nil
		}),
	}
	cmd.Flags().BoolVar(&showSecret, "show-secret", false, "show unmasked secrets")
	return cmd
}

func newServerConfigSetCmd() *cobra.Command {
	var host, port, channelID, cors, apiURL, logLevel, databaseURL, valkeyURL string
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set server.yaml fields (bot tokens prompted interactively)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			f, err := serverconfig.Load(userdir.ServerInstallDir())
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("host") {
				f.Host = host
			}
			if cmd.Flags().Changed("port") {
				f.Port = port
			}
			if cmd.Flags().Changed("channel-id") {
				f.ChannelID = channelID
			}
			if cmd.Flags().Changed("cors") {
				f.CORS = cors
			}
			if cmd.Flags().Changed("api-url") {
				f.APIURL = apiURL
			}
			if cmd.Flags().Changed("log-level") {
				f.LogLevel = logLevel
			}
			if cmd.Flags().Changed("database-url") {
				f.DatabaseURL = databaseURL
			}
			if cmd.Flags().Changed("valkey-url") {
				f.ValkeyURL = valkeyURL
			}
			if !flagJSON && ui.IsTTY(os.Stdin) && len(f.BotTokens) == 0 &&
				!cmd.Flags().Changed("database-url") && !cmd.Flags().Changed("valkey-url") {
				toks, err := promptBotTokens()
				if err != nil {
					return err
				}
				f.BotTokens = toks
			}
			if err := serverconfig.Save(userdir.ServerInstallDir(), f); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"path": serverconfig.Path(userdir.ServerInstallDir())})
			}
			ui.PrintSuccess("Wrote %s", serverconfig.Path(userdir.ServerInstallDir()))
			return nil
		}),
	}
	cmd.Flags().StringVar(&host, "host", "", "listen host")
	cmd.Flags().StringVar(&port, "port", "", "listen port")
	cmd.Flags().StringVar(&channelID, "channel-id", "", "Discord channel ID")
	cmd.Flags().StringVar(&cors, "cors", "", "CORS origin")
	cmd.Flags().StringVar(&apiURL, "api-url", "", "public API URL")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "debug|info|warn|error")
	cmd.Flags().StringVar(&databaseURL, "database-url", "", "advanced: database URL")
	cmd.Flags().StringVar(&valkeyURL, "valkey-url", "", "advanced: Valkey URL")
	return cmd
}

func newServerConfigUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unset <keys...>",
		Short: "Clear server.yaml keys",
		Args:  cobra.MinimumNArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if err := serverconfig.Unset(userdir.ServerInstallDir(), args...); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]any{"unset": args})
			}
			ui.PrintSuccess("Updated %s", serverconfig.Path(userdir.ServerInstallDir()))
			return nil
		}),
	}
}

func newServerConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print canonical server.yaml path",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			path := serverconfig.Path(userdir.ServerInstallDir())
			if flagJSON {
				return writeJSON(map[string]string{
					"config": path,
					"env":    serverconfig.EnvPath(userdir.ServerInstallDir()),
					"dir":    serverconfig.Dir(userdir.ServerInstallDir()),
				})
			}
			fmt.Println(path)
			return nil
		}),
	}
}

func promptServerBasics(opts *localserver.InstallOptions) error {
	f, err := serverconfig.Load(userdir.ServerInstallDir())
	if err != nil {
		return err
	}
	if len(f.BotTokens) == 0 {
		toks, err := promptBotTokens()
		if err != nil {
			return err
		}
		opts.BotTokens = toks
	}
	if strings.TrimSpace(f.ChannelID) == "" {
		ch, err := promptArg("", "Discord channel ID")
		if err != nil {
			return err
		}
		opts.ChannelID = ch
	}
	if strings.TrimSpace(f.GuildID) == "" {
		g, err := promptArg("", "Discord guild ID (optional, empty = 8 MB chunks)")
		if err != nil {
			return err
		}
		opts.GuildID = strings.TrimSpace(g)
	}
	return nil
}

func formatPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	return strconv.Itoa(pid)
}

// logDisplay writes each line flush-left with level colors (not banner-padded).
// PrintLogsDefault / Follow write one full line per Write via Fprintln.
type logDisplay struct{ w io.Writer }

func (d logDisplay) Write(p []byte) (int, error) {
	s := strings.TrimRight(string(p), "\r\n")
	if s != "" {
		fmt.Fprintln(d.w, colorizeLogLine(s))
	}
	return len(p), nil
}

func colorizeLogLine(s string) string {
	on := ui.ColorOn(os.Stdout)
	if !on {
		return s
	}
	// "15:04:05  INFO   msg …" or "INFO   msg …"
	fields := strings.SplitN(s, "  ", 3)
	if len(fields) < 2 {
		return s
	}
	levelIdx := 0
	if len(fields[0]) == 8 && fields[0][2] == ':' { // HH:MM:SS
		if len(fields) < 3 {
			return s
		}
		levelIdx = 1
	}
	level := strings.TrimSpace(fields[levelIdx])
	var colored string
	switch level {
	case "DEBUG":
		colored = ui.Dim(true, level)
	case "INFO":
		colored = ui.Cyan(true, level)
	case "WARN", "WARNING":
		colored = ui.Yellow(true, level)
	case "ERROR":
		colored = ui.Red(true, level)
	default:
		return s
	}
	pad := 5 - len(level)
	if pad < 0 {
		pad = 0
	}
	fields[levelIdx] = colored + strings.Repeat(" ", pad)
	return strings.Join(fields, "  ")
}
