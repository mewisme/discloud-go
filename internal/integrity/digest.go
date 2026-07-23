// Package integrity computes DisCloud whole-file digests (discloud-sha256-v1).
package integrity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
)

// ChunkSize is the DisCloud chunk size (8 MiB). Used by FileSHA256FromReader.
const ChunkSize = 8 << 20

// FileSHA256 computes discloud-sha256-v1 from ordered chunk hashes and sizes.
//
//	n == 1: digest = chunk_hash_0  (equals sha256sum of the file)
//	n  > 1: digest = SHA256( concat for i: BE64(size_i) || raw32(chunk_hash_i) )
//
// hashes are hex-encoded SHA-256 of each chunk's bytes; sizes are those chunk lengths.
func FileSHA256(hashes []string, sizes []int64) (string, error) {
	if len(hashes) == 0 {
		return "", fmt.Errorf("integrity: empty chunk list")
	}
	if len(hashes) != len(sizes) {
		return "", fmt.Errorf("integrity: %d hashes vs %d sizes", len(hashes), len(sizes))
	}
	for i, hexHash := range hashes {
		raw, err := hex.DecodeString(hexHash)
		if err != nil || len(raw) != sha256.Size {
			return "", fmt.Errorf("integrity: invalid chunk hash at %d", i)
		}
		if sizes[i] < 0 {
			return "", fmt.Errorf("integrity: negative size at %d", i)
		}
	}
	if len(hashes) == 1 {
		return hashes[0], nil
	}
	h := sha256.New()
	var be [8]byte
	for i, hexHash := range hashes {
		raw, _ := hex.DecodeString(hexHash)
		binary.BigEndian.PutUint64(be[:], uint64(sizes[i]))
		_, _ = h.Write(be[:])
		_, _ = h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FileSHA256FromReader splits r into ChunkSize pieces, hashes each, and returns FileSHA256.
func FileSHA256FromReader(r io.Reader) (string, error) {
	var hashes []string
	var sizes []int64
	buf := make([]byte, ChunkSize)
	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			sum := sha256.Sum256(buf[:n])
			hashes = append(hashes, hex.EncodeToString(sum[:]))
			sizes = append(sizes, int64(n))
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if len(hashes) == 0 {
		return "", fmt.Errorf("integrity: empty file")
	}
	return FileSHA256(hashes, sizes)
}
