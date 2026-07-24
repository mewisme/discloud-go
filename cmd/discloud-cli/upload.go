package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/mewisme/discloud-go/internal/client"
	"github.com/ncruces/zenity"
	"github.com/spf13/cobra"
)

func newUploadCmd() *cobra.Command {
	var name string
	var workers int
	var abort bool
	cmd := &cobra.Command{
		Use:   "upload [path...]",
		Short: "Resumable chunked upload (session + local checkpoint)",
		Args:  cobra.ArbitraryArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			if abort {
				path, err := promptArg(ui.ArgOrEmpty(args, 0), "Path")
				if err != nil {
					return err
				}
				if len(args) > 1 {
					return fmt.Errorf("--abort accepts a single path")
				}
				c, err := apiClient()
				if err != nil {
					return err
				}
				if err := c.AbortUpload(path); err != nil {
					return err
				}
				ui.PrintSuccess("Aborted upload session for %s", path)
				return nil
			}

			paths := args
			if len(paths) == 0 {
				one, err := promptArg("", "Path")
				if err != nil {
					return err
				}
				paths = []string{one}
			}

			jobs, err := expandUploadPaths(paths)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return fmt.Errorf("no non-empty files to upload")
			}
			if name != "" && len(jobs) != 1 {
				return fmt.Errorf("--name only allowed with a single file")
			}
			if name != "" {
				jobs[0].remoteName = name
			}

			c, err := apiClient()
			if err != nil {
				return err
			}
			return uploadJobs(c, jobs, workers)
		}),
	}
	cmd.Flags().StringVar(&name, "name", "", "remote file name (single file only)")
	cmd.Flags().BoolVar(&abort, "abort", false, "cancel server session for this path's checkpoint")
	cmd.PersistentFlags().IntVar(&workers, "workers", 0, "parallel chunk workers per file")
	cmd.AddCommand(newUploadPickCmd(&workers))
	return cmd
}

func newUploadPickCmd(workers *int) *cobra.Command {
	return &cobra.Command{
		Use:   "pick",
		Short: "Pick files in a dialog, then upload",
		Args:  cobra.NoArgs,
		RunE: runE(func(cmd *cobra.Command, args []string) error {
			paths, err := zenity.SelectFileMultiple(
				zenity.Title("Select files to upload"),
			)
			if errors.Is(err, zenity.ErrCanceled) {
				return nil
			}
			if err != nil {
				return err
			}
			jobs, err := expandUploadPaths(paths)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				return fmt.Errorf("no non-empty files to upload")
			}
			c, err := apiClient()
			if err != nil {
				return err
			}
			return uploadJobs(c, jobs, *workers)
		}),
	}
}

type uploadJob struct {
	path       string
	remoteName string
	size       int64
}

func expandUploadPaths(paths []string) ([]uploadJob, error) {
	var jobs []uploadJob
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			if st.Size() == 0 {
				continue
			}
			jobs = append(jobs, uploadJob{path: p, remoteName: filepath.Base(p), size: st.Size()})
			continue
		}
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Size() == 0 {
				return nil
			}
			rel, err := filepath.Rel(p, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "../") {
				return fmt.Errorf("invalid relative path %s", rel)
			}
			jobs = append(jobs, uploadJob{path: path, remoteName: rel, size: info.Size()})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func uploadJobs(c *client.Client, jobs []uploadJob, workers int) error {
	showUI := ui.ShowWaitUI()
	var prog *ui.UploadProgress
	bars := make([]*ui.FileBar, len(jobs))

	ctx := context.Background()
	if showUI {
		files := make([]ui.ProgressFile, len(jobs))
		for i, job := range jobs {
			files[i] = ui.ProgressFile{Name: job.remoteName, Total: job.size}
		}
		prog = ui.NewUploadProgress(os.Stderr, files)
		ctx = prog.Context()
		for i := range jobs {
			bars[i] = prog.Bar(i)
		}
	} else {
		var stop context.CancelFunc
		ctx, stop = signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
	}

	items := make([]FileItem, 0, len(jobs))
	var firstErr error
	for i, job := range jobs {
		if err := ctx.Err(); err != nil {
			firstErr = err
			for j := i; j < len(bars); j++ {
				if bars[j] != nil {
					bars[j].Abort()
				}
			}
			break
		}
		item, err := uploadOne(ctx, c, job.path, job.remoteName, workers, bars[i])
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", job.remoteName, err)
			}
			for j := i + 1; j < len(bars); j++ {
				if bars[j] != nil {
					bars[j].Abort()
				}
			}
			break
		}
		items = append(items, item)
	}
	if prog != nil {
		prog.Wait()
	}
	if firstErr != nil {
		return firstErr
	}

	if flagJSON {
		return writeJSON(items)
	}
	return printUploadSummary(items)
}

func uploadOne(ctx context.Context, c *client.Client, path, name string, workers int, bar *ui.FileBar) (FileItem, error) {
	var progress client.ProgressFunc
	if bar != nil {
		progress = bar.Update
	}
	raw, err := c.UploadChunked(path, client.UploadChunkedOptions{
		FileName: name,
		Workers:  workers,
		Progress: progress,
		Context:  ctx,
	})
	if err != nil {
		if bar != nil {
			bar.Abort()
		}
		return FileItem{}, err
	}
	if bar != nil {
		bar.Done()
	}
	return decode[FileItem](raw)
}

func printUploadSummary(items []FileItem) error {
	rows := make([]ui.UploadRow, len(items))
	for i, item := range items {
		rows[i] = ui.UploadRow{
			Name:        item.FileName,
			Size:        item.FileSize,
			Status:      item.Status,
			ID:          item.FileID,
			SHA256:      item.SHA256,
			AccessToken: item.AccessToken,
		}
	}
	return ui.PrintUploadTable(os.Stdout, rows)
}
