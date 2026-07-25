package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate with DisCloud",
	}
	cmd.AddCommand(
		newAuthLoginCmd(),
		newAuthLogoutCmd(),
		newAuthSignupCmd(),
		newAuthMeCmd(),
		newAuthPasswordCmd(),
		newAuthTokenCmd(),
	)
	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:     "login [username] [password]",
		Aliases: []string{"signin"},
		Short:   "Sign in (prefer prompt or --password-stdin; positional password deprecated)",
		Args:    cobra.MaximumNArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			userArg, passArg := "", ""
			if len(args) >= 1 {
				userArg = args[0]
			}
			if len(args) >= 2 {
				passArg = args[1]
			}
			username, err := promptUsername(userArg)
			if err != nil {
				return err
			}
			password, err := resolvePassword(passArg, passwordStdin)
			if err != nil {
				return err
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Signing in…", func() (map[string]any, error) {
				return c.SignIn(username, password)
			})
			if err != nil {
				return err
			}
			u, err := decode[AuthUser](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(u)
			}
			name := u.Username
			if name == "" {
				name = username
			}
			ui.PrintSuccess("Logged in as %s", ui.Bold(ui.ColorOn(os.Stdout), name))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin (one line)")
	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "logout",
		Aliases: []string{"signout"},
		Short:   "Sign out and clear local session",
		Args:    cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			if err := ui.WithSpinner("Signing out…", c.SignOut); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "logged out"})
			}
			ui.PrintSuccess("Logged out")
			return nil
		}),
	}
}

func newAuthSignupCmd() *cobra.Command {
	var passwordStdin bool
	cmd := &cobra.Command{
		Use:   "signup [username] [password]",
		Short: "Create an account",
		Args:  cobra.MaximumNArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			userArg, passArg := "", ""
			if len(args) >= 1 {
				userArg = args[0]
			}
			if len(args) >= 2 {
				passArg = args[1]
			}
			username, err := promptUsername(userArg)
			if err != nil {
				return err
			}
			password, err := resolvePassword(passArg, passwordStdin)
			if err != nil {
				return err
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Creating account…", func() (map[string]any, error) {
				return c.SignUp(username, password)
			})
			if err != nil {
				return err
			}
			u, err := decode[AuthUser](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(u)
			}
			name := u.Username
			if name == "" {
				name = username
			}
			ui.PrintSuccess("Signed up as %s", ui.Bold(ui.ColorOn(os.Stdout), name))
			return nil
		}),
	}
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "read password from stdin (one line)")
	return cmd
}

func newAuthMeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "me",
		Short: "Show the current account",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Loading account…", c.Me)
			if err != nil {
				return err
			}
			me, err := decode[MeResponse](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(me)
			}
			printMe(me)
			return nil
		}),
	}
}

func printMe(me MeResponse) {
	title := me.Username
	if me.Role != "" {
		title = fmt.Sprintf("%s (%s)", me.Username, me.Role)
	}
	profile := [][]string{
		{"user", title},
		{"id", me.ID},
		{"created", ui.FormatTimeOrDash(me.CreatedAt)},
	}
	if me.Preferences.DefaultVisibility != "" {
		profile = append(profile, []string{"default", ui.ShortVis(me.Preferences.DefaultVisibility)})
	}
	blocks := []ui.KVBlock{
		{Title: "Profile", Rows: profile},
		{Title: "Stats", Rows: [][]string{
			{"files", fmt.Sprintf("%d  (%d public · %d private)",
				me.Stats.FileCount, me.Stats.PublicCount, me.Stats.PrivateCount)},
			{"storage", client.FormatBytes(me.Stats.TotalBytes)},
			{"expiring soon", fmt.Sprintf("%d", me.Stats.ExpiringSoonCount)},
		}},
	}
	if me.Token != nil && me.Token.Valid {
		scopes := strings.Join(me.Token.Scopes, ",")
		expires := "never"
		if me.Token.ExpiresAt != nil {
			expires = ui.FormatTimeOrDash(*me.Token.ExpiresAt)
		}
		lastUsed := "—"
		if me.Token.LastUsedAt != nil {
			lastUsed = ui.FormatTimeOrDash(*me.Token.LastUsedAt)
		}
		blocks = append(blocks, ui.KVBlock{
			Title: "API token",
			Rows: [][]string{
				{"status", "valid"},
				{"id", me.Token.ID},
				{"name", me.Token.Name},
				{"scopes", scopes},
				{"created", ui.FormatTimeOrDash(me.Token.CreatedAt)},
				{"expires", expires},
				{"last used", lastUsed},
			},
		})
	} else {
		session := [][]string{
			{"since", ui.FormatTimeOrDash(me.Session.CreatedAt)},
			{"last seen", ui.FormatTimeOrDash(me.Session.LastSeenAt)},
			{"expires", ui.FormatTimeOrDash(me.Session.ExpiresAt)},
		}
		if me.Session.IP != "" {
			session = append(session, []string{"ip", me.Session.IP})
		}
		if me.Session.UserAgent != "" {
			session = append(session, []string{"ua", me.Session.UserAgent})
		}
		blocks = append(blocks, ui.KVBlock{Title: "Session", Rows: session})
	}
	_ = ui.PrintKVBlocks(os.Stdout, blocks)
}

func newAuthPasswordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "password [current] [new]",
		Short: "Change password (positional deprecated; prompts when missing)",
		Args:  cobra.MaximumNArgs(2),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			curArg, newArg := "", ""
			if len(args) >= 1 {
				curArg = args[0]
			}
			if len(args) >= 2 {
				newArg = args[1]
			}
			current, err := resolvePassword(curArg, false)
			if err != nil {
				return fmt.Errorf("current password: %w", err)
			}
			var next string
			if newArg != "" {
				next = newArg
			} else {
				next, err = promptPassword("New password")
				if err != nil {
					return err
				}
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			if err := ui.WithSpinner("Updating password…", func() error {
				return c.ChangePassword(current, next)
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "password updated"})
			}
			ui.PrintSuccess("Password updated")
			return nil
		}),
	}
	return cmd
}

func newAuthTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Manage personal access tokens",
	}
	cmd.AddCommand(
		newAuthTokenCreateCmd(),
		newAuthTokenListCmd(),
		newAuthTokenInfoCmd(),
		newAuthTokenRevokeCmd(),
	)
	return cmd
}

func newAuthTokenCreateCmd() *cobra.Command {
	var name, scopesCSV, expires string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a personal access token (raw secret shown once)",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			needPrompt := strings.TrimSpace(name) == "" || strings.TrimSpace(scopesCSV) == ""
			var err error
			name, err = promptArg(name, "Token name")
			if err != nil {
				return err
			}
			scopes, err := promptTokenScopes(scopesCSV)
			if err != nil {
				return err
			}
			if !cmd.Flags().Changed("expires") && needPrompt {
				expires, err = promptTokenExpires()
				if err != nil {
					return err
				}
			} else {
				expires = strings.TrimSpace(expires)
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			var out map[string]any
			if err := ui.WithSpinner("Creating token…", func() error {
				var e error
				out, e = c.CreateAPIToken(name, scopes, expires)
				return e
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(out)
			}
			raw, _ := out["token"].(string)
			id, _ := out["id"].(string)
			if err := ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
				Title: "API token",
				Rows: [][]string{
					{"id", id},
					{"name", name},
					{"scopes", strings.Join(scopes, ",")},
					{"token", raw},
				},
			}}); err != nil {
				return err
			}
			ui.PrintSuccess("Token created (copy the secret now; it will not be shown again)")
			return nil
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "Token label (prompted if omitted)")
	cmd.Flags().StringVar(&scopesCSV, "scopes", "", "Comma-separated scopes: upload,read,manage,admin (prompted if omitted)")
	cmd.Flags().StringVar(&expires, "expires", "", "Optional RFC3339 expiry (max 1 year; prompted interactively when creating without flags)")
	return cmd
}

func promptTokenScopes(csv string) ([]string, error) {
	if scopes := splitCSV(csv); len(scopes) > 0 {
		return scopes, nil
	}
	if flagJSON {
		return nil, fmt.Errorf("--scopes is required with --json")
	}
	if !ui.IsTTY(os.Stdin) {
		return nil, fmt.Errorf("--scopes is required (upload,read,manage,admin)")
	}
	var picked []string
	err := runHuh(huh.NewMultiSelect[string]().
		Title("Scopes").
		Description("Space to toggle, Enter to confirm").
		Options(
			huh.NewOption("upload", "upload"),
			huh.NewOption("read", "read"),
			huh.NewOption("manage", "manage"),
			huh.NewOption("admin", "admin"),
		).
		Filterable(false).
		Height(6).
		Value(&picked).
		Validate(func(s []string) error {
			if len(s) == 0 {
				return fmt.Errorf("pick at least one scope")
			}
			return nil
		}))
	if err != nil {
		return nil, err
	}
	return picked, nil
}

func promptTokenExpires() (string, error) {
	if flagJSON || !ui.IsTTY(os.Stdin) {
		return "", nil
	}
	var exp string
	err := runHuh(huh.NewInput().
		Title("Expires (optional)").
		Description("RFC3339 timestamp, or leave empty for no expiry").
		Placeholder("never").
		Value(&exp))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(exp), nil
}

func newAuthTokenListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List personal access tokens",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			var out map[string]any
			if err := ui.WithSpinner("Listing tokens…", func() error {
				var e error
				out, e = c.ListAPITokens()
				return e
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(out)
			}
			tokens, _ := out["tokens"].([]any)
			if len(tokens) == 0 {
				ui.PrintInfo("No API tokens")
				return nil
			}
			blocks := make([]ui.KVBlock, 0, len(tokens))
			for _, raw := range tokens {
				t, _ := raw.(map[string]any)
				scopes := ""
				if s, ok := t["scopes"].([]any); ok {
					parts := make([]string, 0, len(s))
					for _, v := range s {
						parts = append(parts, fmt.Sprint(v))
					}
					scopes = strings.Join(parts, ",")
				}
				blocks = append(blocks, ui.KVBlock{
					Title: fmt.Sprint(t["name"]),
					Rows: [][]string{
						{"id", fmt.Sprint(t["id"])},
						{"scopes", scopes},
						{"created", fmt.Sprint(t["createdAt"])},
					},
				})
			}
			return ui.PrintKVBlocks(os.Stdout, blocks)
		}),
	}
}

func newAuthTokenInfoCmd() *cobra.Command {
	var prompt bool
	cmd := &cobra.Command{
		Use:     "info",
		Aliases: []string{"validate", "whoami"},
		Short:   "Validate a PAT and show its info as a table (Bearer only; ignores session cookies)",
		Args:    cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			cfg := client.DefaultConfig()
			if flagBase != "" {
				cfg.BaseURL = strings.TrimRight(flagBase, "/")
			}
			if flagOrigin != "" {
				cfg.Origin = strings.TrimRight(flagOrigin, "/")
			}

			tok := strings.TrimSpace(cfg.Token)
			if prompt || tok == "" {
				if flagJSON && tok == "" {
					return fmt.Errorf("no token configured; set DISCLOUD_TOKEN, run config set token, or omit --json to prompt")
				}
				if tok == "" || prompt {
					if flagJSON && prompt {
						return fmt.Errorf("--prompt is interactive; omit --json")
					}
					var err error
					tok, err = promptPassword("Token")
					if err != nil {
						return err
					}
				}
			}
			if !strings.HasPrefix(tok, "dc_") {
				return fmt.Errorf("not a personal access token (want dc_… prefix)")
			}
			cfg.Token = tok
			// Never attach a cookie jar for token validation.
			cfg.CookiePath = ""

			c, err := client.New(cfg)
			if err != nil {
				return err
			}
			raw, err := ui.WaitVal("Validating token…", c.Me)
			if err != nil {
				var api *client.Error
				if errors.As(err, &api) && api.Status == 401 {
					return fmt.Errorf("invalid, revoked, or expired token (or API at %s has no PAT support yet — deploy Phase 3)", cfg.BaseURL)
				}
				return err
			}
			me, err := decode[MeResponse](raw)
			if err != nil {
				return err
			}
			if flagJSON {
				out := map[string]any{"valid": me.Token != nil && me.Token.Valid, "account": me}
				if me.Token != nil {
					out["token"] = me.Token
				}
				return writeJSON(out)
			}
			if me.Token == nil || !me.Token.Valid {
				return fmt.Errorf("invalid, revoked, or expired token")
			}
			scopes := strings.Join(me.Token.Scopes, ",")
			expires := "never"
			if me.Token.ExpiresAt != nil {
				expires = ui.FormatTimeOrDash(*me.Token.ExpiresAt)
			}
			lastUsed := "—"
			if me.Token.LastUsedAt != nil {
				lastUsed = ui.FormatTimeOrDash(*me.Token.LastUsedAt)
			}
			return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{
				{
					Title: "Token",
					Rows: [][]string{
						{"status", "valid"},
						{"id", me.Token.ID},
						{"name", me.Token.Name},
						{"scopes", scopes},
						{"created", ui.FormatTimeOrDash(me.Token.CreatedAt)},
						{"expires", expires},
						{"last used", lastUsed},
						{"preview", maskAPIToken(tok)},
					},
				},
				{
					Title: "Account",
					Rows: [][]string{
						{"user", me.Username},
						{"role", me.Role},
						{"id", me.ID},
					},
				},
			})
		}),
	}
	cmd.Flags().BoolVar(&prompt, "prompt", false, "Prompt for token instead of using config/env")
	return cmd
}

func newAuthTokenRevokeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "revoke <id>",
		Short: "Revoke a personal access token",
		Args:  cobra.ExactArgs(1),
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if !yes {
				if err := promptConfirm(fmt.Sprintf("Revoke token %s?", id)); err != nil {
					return err
				}
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			if err := ui.WithSpinner("Revoking token…", func() error {
				return c.RevokeAPIToken(id)
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "revoked", "id": id})
			}
			ui.PrintSuccess("Token revoked")
			return nil
		}),
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func promptUsername(arg string) (string, error) {
	return promptArg(arg, "Username")
}

func promptArg(arg, title string) (string, error) {
	if v := strings.TrimSpace(arg); v != "" {
		return v, nil
	}
	if flagJSON {
		return "", fmt.Errorf("%s is required with --json", title)
	}
	if !ui.IsTTY(os.Stdin) {
		return "", fmt.Errorf("%s is required", title)
	}
	var v string
	err := runHuh(huh.NewInput().
		Title(title).
		Value(&v).
		Validate(func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s is required", title)
			}
			return nil
		}))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(v), nil
}

// resolvePassword picks password from --password-stdin, deprecated positional, or TTY prompt.
func resolvePassword(positional string, passwordStdin bool) (string, error) {
	if positional != "" && passwordStdin {
		return "", fmt.Errorf("conflicting password inputs: positional password and --password-stdin")
	}
	if passwordStdin {
		return readPasswordStdin(os.Stdin)
	}
	if positional != "" {
		return positional, nil
	}
	return promptPassword("Password")
}

func promptPassword(title string) (string, error) {
	if flagJSON {
		return "", fmt.Errorf("%s required with --json", title)
	}
	if !ui.IsTTY(os.Stdin) {
		return "", fmt.Errorf("password required: use --password-stdin or run in a TTY")
	}
	var pw string
	err := runHuh(huh.NewInput().
		Title(title).
		EchoMode(huh.EchoModePassword).
		Value(&pw).
		Validate(func(s string) error {
			if s == "" {
				return errors.New("Password is required")
			}
			return nil
		}))
	if err != nil {
		return "", err
	}
	return pw, nil
}

func promptConfirm(title string) error {
	if flagJSON || !ui.IsTTY(os.Stdin) {
		return fmt.Errorf("refusing without -y (non-interactive)")
	}
	var ok bool
	if err := runHuh(huh.NewConfirm().
		Title(title).
		Affirmative("Yes").
		Negative("No").
		Value(&ok)); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("aborted")
	}
	return nil
}

func readPasswordStdin(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if len(line) == 0 && err == io.EOF {
		return "", fmt.Errorf("empty password from stdin")
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return "", fmt.Errorf("empty password from stdin")
	}
	return line, nil
}
