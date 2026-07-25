package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/mewisme/discloud-go/internal/discord"
	"github.com/mewisme/discloud-go/internal/selfmanaged"
	"github.com/mewisme/discloud-go/internal/transfer"
)

func isBundleArg(arg string) bool {
	if arg == "" {
		return false
	}
	st, err := os.Stat(arg)
	if err != nil || st.IsDir() {
		return false
	}
	return selfmanaged.DetectBundle(arg)
}

func errServerOnly() error {
	return fmt.Errorf("This operation is only available for server-managed files.")
}

func filesListBundle(path string) error {
	meta, _, err := loadBundleMeta(path, "", false)
	if err != nil {
		return err
	}
	if flagJSON {
		return writeJSON(meta)
	}
	blocks := []ui.KVBlock{{
		Title: "Bundle",
		Rows: [][]string{
			{"bundle id", meta.BundleID},
			{"protection", meta.Protection},
			{"files", fmt.Sprintf("%d", meta.FileCount)},
			{"total size", client.FormatBytes(meta.TotalSize)},
			{"premium tier", fmt.Sprintf("%d", meta.PremiumTier)},
			{"chunk size", transfer.FormatChunkMB(meta.ChunkSize)},
			{"created", meta.CreatedAt.UTC().Format(time.RFC3339)},
		},
	}}
	for _, f := range meta.Files {
		blocks = append(blocks, ui.KVBlock{
			Title: f.OriginalName,
			Rows: [][]string{
				{"id", f.ID},
				{"path", f.LogicalPath},
				{"size", client.FormatBytes(f.Size)},
				{"chunks", fmt.Sprintf("%d", f.ChunkCount)},
				{"mime", f.MIMEType},
				{"digest", f.SHA256},
				{"uploaded", f.UploadedAt.UTC().Format(time.RFC3339)},
			},
		})
	}
	return ui.PrintKVBlocks(os.Stdout, blocks)
}

func filesInspectBundle(path string, selectors []string) error {
	meta, _, err := loadBundleMeta(path, "", false)
	if err != nil {
		return err
	}
	files := meta.Files
	if len(selectors) > 0 || len(files) > 1 {
		files, err = selectBundleFiles(meta.Files, selectors, len(selectors) == 0 && len(files) == 1, false)
		if err != nil {
			return err
		}
	}
	if flagJSON {
		return writeJSON(map[string]any{"bundle": meta, "selected": files})
	}
	shortID := meta.ConfigIdentity
	if len(shortID) > 12 {
		shortID = shortID[:12] + "…"
	}
	blocks := []ui.KVBlock{{
		Title: "Bundle",
		Rows: [][]string{
			{"bundle id", meta.BundleID},
			{"protection", meta.Protection},
			{"guild id", meta.GuildID},
			{"channel id", meta.ChannelID},
			{"config id", shortID},
			{"premium tier", fmt.Sprintf("%d", meta.PremiumTier)},
			{"chunk size", transfer.FormatChunkMB(meta.ChunkSize)},
			{"files", fmt.Sprintf("%d", meta.FileCount)},
			{"total size", client.FormatBytes(meta.TotalSize)},
			{"total chunks", fmt.Sprintf("%d", meta.TotalChunks)},
		},
	}}
	for _, f := range files {
		blocks = append(blocks, ui.KVBlock{
			Title: "File " + f.OriginalName,
			Rows: [][]string{
				{"id", f.ID},
				{"logical path", f.LogicalPath},
				{"size", client.FormatBytes(f.Size)},
				{"chunks", fmt.Sprintf("%d", f.ChunkCount)},
				{"sha256", f.SHA256},
				{"mime", f.MIMEType},
			},
		})
	}
	return ui.PrintKVBlocks(os.Stdout, blocks)
}

func filesVerifyBundle(bundlePath, localPath string, selectors []string, all bool) error {
	meta, _, err := loadBundleMeta(bundlePath, "", false)
	if err != nil {
		return err
	}
	// Structure-only verify
	if localPath == "" {
		if meta.FileCount != len(meta.Files) {
			return fmt.Errorf("file count mismatch")
		}
		for _, f := range meta.Files {
			if len(f.Chunks) != f.ChunkCount && f.ChunkCount != 0 {
				return fmt.Errorf("%s: chunk count mismatch", f.OriginalName)
			}
		}
		if flagJSON {
			return writeJSON(map[string]any{"ok": true, "bundleId": meta.BundleID})
		}
		ui.PrintSuccess("bundle structure ok: %s", bundlePath)
		return nil
	}

	selected, err := selectBundleFiles(meta.Files, selectors, all || len(meta.Files) == 1, false)
	if err != nil {
		return err
	}
	if len(selected) != 1 && !all {
		return fmt.Errorf("select one file with --file for local verify")
	}
	if all {
		return fmt.Errorf("local verify with --all is not supported; verify one file at a time")
	}
	f := selected[0]
	got, err := transfer.HashFile(localPath)
	if err != nil {
		return err
	}
	st, err := os.Stat(localPath)
	if err != nil {
		return err
	}
	if st.Size() != f.Size || !strings.EqualFold(got, f.SHA256) {
		return fmt.Errorf("verify failed: size/digest mismatch")
	}
	if flagJSON {
		return writeJSON(map[string]any{"ok": true, "fileId": f.ID, "sha256": got})
	}
	ui.PrintSuccess("%s matches bundle file %s", localPath, f.OriginalName)
	return nil
}

func filesDeleteBundle(path string, selectors []string, all, yes, localOnly, keepDataFile bool, password string, passwordStdin bool) error {
	meta, pw, err := loadBundleMeta(path, password, passwordStdin)
	if err != nil {
		return err
	}
	selected, err := selectBundleFiles(meta.Files, selectors, all || len(meta.Files) == 1, false)
	if err != nil {
		return err
	}
	if !yes {
		if flagJSON || !ui.IsTTY(os.Stdin) {
			return fmt.Errorf("refusing without -y (non-interactive)")
		}
		names := make([]string, len(selected))
		for i, f := range selected {
			names[i] = f.OriginalName
		}
		if err := promptConfirm(fmt.Sprintf("Delete %d file(s) from bundle (%s)?", len(selected), strings.Join(names, ", "))); err != nil {
			return err
		}
	}

	var delRes selfmanaged.DeleteResult
	if !localOnly {
		su, err := client.LoadSelfUpload()
		if err != nil {
			return err
		}
		if !su.Configured() {
			return fmt.Errorf("self-upload is not configured")
		}
		want := selfmanaged.ComputeConfigIdentity(su.GuildID, su.ChannelID, su.BotTokens)
		if !selfmanaged.IdentitiesMatch(want, meta.ConfigIdentity) {
			return selfmanaged.ErrIdentityMismatch
		}
		dc := discord.NewFromTokens(su.BotTokens, su.ChannelID)
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		delRes = selfmanaged.DeleteRemoteMessages(ctx, dc, selected)
	}

	removeIDs := map[string]struct{}{}
	for _, f := range selected {
		removeIDs[f.ID] = struct{}{}
	}
	if len(removeIDs) == len(meta.Files) {
		if err := selfmanaged.RemoveLocalBundle(path, keepDataFile); err != nil {
			return err
		}
	} else {
		if err := selfmanaged.RewriteBundleAfterDelete(path, meta, removeIDs, pw); err != nil {
			return err
		}
	}

	if flagJSON {
		return writeJSON(map[string]any{
			"deletedRemote": delRes,
			"removedFiles":  len(selected),
			"dataFile":      path,
			"keptDataFile":  keepDataFile,
		})
	}
	ui.PrintSuccess("Removed %d file(s) from bundle", len(selected))
	if !localOnly {
		ui.PrintUnderBanner(os.Stderr, fmt.Sprintf(
			"Discord: deleted=%d missing=%d unauthorized=%d failed=%d",
			delRes.Deleted, delRes.Missing, delRes.Unauthorized, delRes.Failed))
	}
	return nil
}
