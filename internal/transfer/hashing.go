package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

// HashBytes returns lowercase hex SHA-256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashFile returns lowercase hex SHA-256 of the whole file.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ChunkCount returns how many chunks of chunkSize cover fileSize.
func ChunkCount(fileSize, chunkSize int64) int {
	if fileSize <= 0 || chunkSize <= 0 {
		return 0
	}
	return int((fileSize + chunkSize - 1) / chunkSize)
}

// ReadChunk reads chunk idx from path using chunkSize.
func ReadChunk(path string, idx int, chunkSize, fileSize int64) ([]byte, error) {
	if chunkSize <= 0 || idx < 0 {
		return nil, fmt.Errorf("invalid chunk params")
	}
	off := int64(idx) * chunkSize
	if off >= fileSize {
		return nil, fmt.Errorf("chunk %d past end of file", idx)
	}
	n := chunkSize
	if rem := fileSize - off; rem < n {
		n = rem
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n)
	if _, err := f.ReadAt(buf, off); err != nil {
		return nil, err
	}
	return buf, nil
}
