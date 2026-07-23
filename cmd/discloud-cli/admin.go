package main

import (
	"fmt"
	"os"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/spf13/cobra"
)

func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Instance admin (requires admin role)",
	}
	cmd.AddCommand(newAdminOverviewCmd())
	return cmd
}

func newAdminOverviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "overview",
		Short: "Show storage, users, uploads, traffic, and dependency health",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			c, err := apiClient()
			if err != nil {
				return err
			}
			ov, err := ui.WaitVal("Loading admin overview…", c.GetAdminOverview)
			if err != nil {
				return err
			}
			if flagJSON {
				return writeJSON(ov)
			}
			dep := func(ok bool) string {
				if ok {
					return "ok"
				}
				return "down"
			}
			return ui.PrintKVBlocks(os.Stdout, []ui.KVBlock{
				{Title: "Storage", Rows: [][]string{
					{"files", fmt.Sprintf("%d", ov.Storage.FileCount)},
					{"bytes", client.FormatBytes(ov.Storage.TotalBytes)},
					{"chunks", fmt.Sprintf("%d", ov.Storage.ChunkStoreCount)},
				}},
				{Title: "Users", Rows: [][]string{
					{"count", fmt.Sprintf("%d", ov.Users.Count)},
					{"admins", fmt.Sprintf("%d", ov.Users.Admins)},
					{"bots", fmt.Sprintf("%d", ov.Bots.Configured)},
				}},
				{Title: "Uploads (24h)", Rows: [][]string{
					{"open", fmt.Sprintf("%d", ov.Uploads.OpenSessions)},
					{"completed", fmt.Sprintf("%d", ov.Uploads.Completed24h)},
					{"expired", fmt.Sprintf("%d", ov.Uploads.Expired24h)},
					{"cancelled", fmt.Sprintf("%d", ov.Uploads.Cancelled24h)},
				}},
				{Title: "Traffic (lifetime)", Rows: [][]string{
					{"downloads", fmt.Sprintf("%d", ov.Traffic.Downloads)},
					{"bytes served", client.FormatBytes(ov.Traffic.BytesServed)},
				}},
				{Title: "Dependencies", Rows: [][]string{
					{"postgres", dep(ov.Deps.Postgres)},
					{"valkey", dep(ov.Deps.Valkey)},
				}},
			})
		}),
	}
}
