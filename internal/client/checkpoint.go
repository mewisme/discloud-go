package client

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type uploadCheckpoint struct {
	UploadID    string         `json:"uploadId"`
	ResumeToken string         `json:"resumeToken"`
	Path        string         `json:"path"`
	Size        int64          `json:"size"`
	ModTimeUnix int64          `json:"modTimeUnix"`
	ChunkSize   int64          `json:"chunkSize"`
	FileName    string         `json:"fileName"`
	Hashes      map[int]string `json:"hashes"`
}

func uploadsDir() string {
	return filepath.Join(defaultConfigDir(), "uploads")
}

func checkpointPath(absPath string, size, mtime int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", absPath, size, mtime)))
	return filepath.Join(uploadsDir(), hex.EncodeToString(sum[:])+".json")
}

func loadCheckpoint(path string, size, mtime int64) (*uploadCheckpoint, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	data, err := os.ReadFile(checkpointPath(abs, size, mtime))
	if err != nil {
		return nil, err
	}
	var cp uploadCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, err
	}
	if cp.Path != abs || cp.Size != size || cp.ModTimeUnix != mtime {
		return nil, fmt.Errorf("checkpoint mismatch")
	}
	if cp.Hashes == nil {
		cp.Hashes = map[int]string{}
	}
	return &cp, nil
}

func saveCheckpoint(cp *uploadCheckpoint) error {
	if err := os.MkdirAll(uploadsDir(), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(checkpointPath(cp.Path, cp.Size, cp.ModTimeUnix), data, 0o600)
}

func clearCheckpoint(path string, size, mtime int64) {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	_ = os.Remove(checkpointPath(abs, size, mtime))
}

func fileFingerprint(st os.FileInfo) (size, mtime int64) {
	return st.Size(), st.ModTime().UTC().Unix()
}
