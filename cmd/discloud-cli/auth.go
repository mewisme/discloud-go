package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

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
			username, err := ui.ResolveUsername(userArg)
			if err != nil {
				return err
			}
			password, err := ui.ResolvePassword(passArg, passwordStdin)
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
			username, err := ui.ResolveUsername(userArg)
			if err != nil {
				return err
			}
			password, err := ui.ResolvePassword(passArg, passwordStdin)
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
			current, err := ui.ResolvePassword(curArg, false)
			if err != nil {
				return fmt.Errorf("current password: %w", err)
			}
			var next string
			if newArg != "" {
				next = newArg
			} else {
				if !ui.IsTTY(os.Stdin) {
					return fmt.Errorf("new password required")
				}
				next, err = ui.ReadPasswordPrompt("New password: ")
				if err != nil {
					return err
				}
				if next == "" {
					return fmt.Errorf("empty password")
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
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("--name is required")
			}
			scopes := splitCSV(scopesCSV)
			if len(scopes) == 0 {
				return fmt.Errorf("--scopes is required (upload,read,manage,admin)")
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
			ui.PrintSuccess("Token created (copy the secret now; it will not be shown again)")
			return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{{
				Title: "API token",
				Rows: [][]string{
					{"id", id},
					{"name", name},
					{"scopes", strings.Join(scopes, ",")},
					{"token", raw},
				},
			}})
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "Token label")
	cmd.Flags().StringVar(&scopesCSV, "scopes", "", "Comma-separated scopes: upload,read,manage,admin")
	cmd.Flags().StringVar(&expires, "expires", "", "Optional RFC3339 expiry (max 1 year)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("scopes")
	return cmd
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
					tok, err = ui.ReadPasswordPrompt("Token: ")
					if err != nil {
						return err
					}
					tok = strings.TrimSpace(tok)
					if tok == "" {
						return fmt.Errorf("token is required")
					}
					ui.ClearLinesUp(os.Stderr, 1)
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
			ui.PrintSuccess("Token valid")
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
				if !ui.IsTTY(os.Stdin) {
					return fmt.Errorf("refusing revoke without -y (non-interactive)")
				}
				if !ui.Confirm(os.Stderr, os.Stdin, fmt.Sprintf("Revoke token %s?", id)) {
					return fmt.Errorf("aborted")
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
