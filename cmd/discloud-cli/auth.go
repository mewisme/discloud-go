package main

import (
	"fmt"
	"os"

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
	_ = ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{
		{Title: "Profile", Rows: profile},
		{Title: "Stats", Rows: [][]string{
			{"files", fmt.Sprintf("%d  (%d public · %d private)",
				me.Stats.FileCount, me.Stats.PublicCount, me.Stats.PrivateCount)},
			{"storage", client.FormatBytes(me.Stats.TotalBytes)},
			{"expiring soon", fmt.Sprintf("%d", me.Stats.ExpiringSoonCount)},
		}},
		{Title: "Session", Rows: session},
	})
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
