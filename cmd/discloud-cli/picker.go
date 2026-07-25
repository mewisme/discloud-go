package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"charm.land/huh/v2"
	"github.com/mewisme/discloud-go/cmd/discloud-cli/ui"
	"github.com/ncruces/zenity"
)

func promptUploadPickMode() (string, error) {
	if flagJSON || !ui.IsTTY(os.Stdin) {
		return "", fmt.Errorf("upload pick requires an interactive TTY")
	}
	var mode string
	if err := huh.NewSelect[string]().
		Title("What do you want to upload?").
		Options(
			huh.NewOption("Files", "files"),
			huh.NewOption("Folder", "folder"),
		).
		Value(&mode).
		Run(); err != nil {
		return "", err
	}
	return mode, nil
}

func pickUploadFiles() ([]string, error) {
	const what = "files to upload"
	ui.WaitPicker(what)
	ui.Selecting(what)
	paths, err := zenity.SelectFileMultiple(
		zenity.Title("Select files to upload"),
	)
	if errors.Is(err, zenity.ErrCanceled) {
		ui.ClearSelecting()
		return nil, err
	}
	if err != nil {
		ui.ClearSelecting()
		return nil, err
	}
	ui.Selected(what, paths...)
	return paths, nil
}

func pickUploadFolder() (string, error) {
	const what = "folder to upload"
	ui.WaitPicker(what)
	ui.Selecting(what)
	dir, err := zenity.SelectFile(
		zenity.Title("Select folder to upload"),
		zenity.Directory(),
	)
	if errors.Is(err, zenity.ErrCanceled) {
		ui.ClearSelecting()
		return "", err
	}
	if err != nil {
		ui.ClearSelecting()
		return "", err
	}
	ui.Selected(what, dir)
	return dir, nil
}

func pickBundleFile() (string, error) {
	const what = ".discloud file"
	ui.WaitPicker(what)
	ui.Selecting(what)
	path, err := zenity.SelectFile(
		zenity.Title("Select .discloud file"),
		zenity.FileFilter{
			Name:     "DisCloud bundle",
			Patterns: []string{"*.discloud"},
			CaseFold: true,
		},
	)
	if errors.Is(err, zenity.ErrCanceled) {
		ui.ClearSelecting()
		return "", err
	}
	if err != nil {
		ui.ClearSelecting()
		return "", err
	}
	ui.Selected(what, path)
	return path, nil
}

func pickDownloadDir() (string, error) {
	const what = "download folder"
	ui.WaitPicker(what)
	ui.Selecting(what)
	dir, err := zenity.SelectFile(
		zenity.Title("Select download folder"),
		zenity.Directory(),
	)
	if errors.Is(err, zenity.ErrCanceled) {
		ui.ClearSelecting()
		return "", err
	}
	if err != nil {
		ui.ClearSelecting()
		return "", err
	}
	ui.Selected(what, dir)
	return dir, nil
}

// expandFolderJobs walks root and builds jobs with logical names
// "basename(root)/rel" so download recreates the folder tree.
// depth 0 = unlimited; depth N matches find -maxdepth (1 = top-level files only).
func expandFolderJobs(root string, depth int) ([]uploadJob, error) {
	st, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !st.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", root)
	}
	base := filepath.Base(root)
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "upload"
	}
	var jobs []uploadJob
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skip, err := folderWalkSkip(root, path, d, depth); skip {
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "../") || rel == ".." {
			return fmt.Errorf("invalid relative path %s", rel)
		}
		logical := base + "/" + rel
		jobs = append(jobs, uploadJob{path: path, remoteName: logical, size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

// folderWalkSkip enforces --depth. When skip is true, return err to the WalkDir
// callback (nil or filepath.SkipDir).
func folderWalkSkip(root, path string, d fs.DirEntry, depth int) (skip bool, err error) {
	if depth <= 0 || path == root {
		return false, nil
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return true, err
	}
	rel = filepath.ToSlash(rel)
	dpt := strings.Count(rel, "/") + 1
	if dpt <= depth {
		return false, nil
	}
	if d.IsDir() {
		return true, filepath.SkipDir
	}
	return true, nil
}
