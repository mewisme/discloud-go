package localserver

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// maxEntryBytes caps a single archive member so a malicious release cannot fill
// the disk during extraction.
const maxEntryBytes = 512 << 20

// Extract unpacks a .zip or .tar.gz archive into destDir, rejecting any member
// whose path would escape destDir.
func Extract(archivePath, destDir string) error {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return err
	}
	switch {
	case strings.HasSuffix(archivePath, ".zip"):
		return extractZip(archivePath, destDir)
	case strings.HasSuffix(archivePath, ".tar.gz"), strings.HasSuffix(archivePath, ".tgz"):
		return extractTarGz(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive %q", filepath.Base(archivePath))
	}
}

func extractZip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, entry := range zr.File {
		target, err := safeJoin(destDir, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if !entry.Mode().IsRegular() {
			continue // symlinks and devices are never part of a release archive
		}
		rc, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeEntry(target, rc, entry.Mode())
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeEntry(target, tr, fs.FileMode(hdr.Mode).Perm()); err != nil {
				return err
			}
		default:
			continue
		}
	}
}

func writeEntry(target string, r io.Reader, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o600
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxEntryBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if n > maxEntryBytes {
		_ = os.Remove(target)
		return fmt.Errorf("archive entry %q exceeds %d bytes", filepath.Base(target), int64(maxEntryBytes))
	}
	return nil
}

// safeJoin resolves an archive member name inside dest, rejecting absolute
// paths, drive letters, and any ".." traversal.
func safeJoin(dest, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", fmt.Errorf("archive entry has an empty name")
	}
	for _, part := range strings.Split(strings.ReplaceAll(name, `\`, "/"), "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe archive entry %q", name)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.HasPrefix(clean, string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	target := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe archive entry %q", name)
	}
	return target, nil
}

// RenameServerBinary finds the extracted server executable anywhere under dir
// and moves it to dir/<BinaryName()> so it never shadows the CLI.
func RenameServerBinary(dir string) (string, error) {
	want := BinaryName()
	candidates := map[string]bool{want: true, "discloud": true, "discloud.exe": true}

	var found string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return err
		}
		if candidates[d.Name()] {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("no discloud server binary in extracted archive")
	}
	target := filepath.Join(dir, want)
	if found != target {
		if err := os.Rename(found, target); err != nil {
			return "", err
		}
	}
	if err := os.Chmod(target, 0o700); err != nil {
		return "", err
	}
	return target, nil
}
