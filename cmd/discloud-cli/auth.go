package main

import (
	"fmt"
	"os"

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
			username, err := resolveUsername(userArg)
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
			raw, err := waitVal("Signing in…", func() (map[string]any, error) {
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
			printSuccess("Logged in as %s", bold(colorOn(os.Stdout), name))
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
			if err := withSpinner("Signing out…", c.SignOut); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "logged out"})
			}
			printSuccess("Logged out")
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
			username, err := resolveUsername(userArg)
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
			raw, err := waitVal("Creating account…", func() (map[string]any, error) {
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
			printSuccess("Signed up as %s", bold(colorOn(os.Stdout), name))
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
			raw, err := waitVal("Loading account…", c.Me)
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
			on := colorOn(os.Stdout)
			fmt.Printf("%s %s\n", dim(on, "id:"), cyan(on, me.ID))
			fmt.Printf("%s %s\n", dim(on, "username:"), bold(on, me.Username))
			if me.Role != "" {
				fmt.Printf("%s %s\n", dim(on, "role:"), me.Role)
			}
			if me.Preferences.DefaultVisibility != "" {
				fmt.Printf("%s %s\n", dim(on, "default:"), visibilityLabel(on, me.Preferences.DefaultVisibility))
			}
			return nil
		}),
	}
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
				if !isTTY(os.Stdin) {
					return fmt.Errorf("new password required")
				}
				next, err = readPasswordPrompt("New password: ")
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
			if err := withSpinner("Updating password…", func() error {
				return c.ChangePassword(current, next)
			}); err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(map[string]string{"status": "password updated"})
			}
			printSuccess("Password updated")
			return nil
		}),
	}
	return cmd
}
