package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	defaultChunkSize = 8 << 20
	defaultWorkers   = 3
	maxWorkers       = 32
	chunkAttempts    = 3
)

// ProgressFunc reports upload progress (sent, total bytes).
type ProgressFunc func(sent, total int64)

// UploadChunkedOptions configures resumable upload.
type UploadChunkedOptions struct {
	FileName string
	Workers  int // 0 → defaultWorkers
	Progress ProgressFunc
}

// UploadRaw POSTs the whole file to /api/upload (non-chunked).
func (c *Client) UploadRaw(path, fileName string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if fileName == "" {
		fileName = filepath.Base(path)
	}
	q := url.Values{"fileName": {fileName}}
	res, err := c.Do(http.MethodPost, "/api/upload?"+q.Encode(), f, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out map[string]any
	if err := decodeResponse(res, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// UploadChunked splits the file, skips known chunks, uploads in parallel, then completes.
func (c *Client) UploadChunked(path string, opt UploadChunkedOptions) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() == 0 {
		return nil, fmt.Errorf("file is empty")
	}
	fileName := opt.FileName
	if fileName == "" {
		fileName = filepath.Base(path)
	}

	info, err := c.GetInfo()
	if err != nil {
		info = Info{ChunkSize: defaultChunkSize}
	}
	chunkSize := info.ChunkSize
	if chunkSize <= 0 {
		chunkSize = defaultChunkSize
	}
	workers := opt.Workers
	if workers <= 0 {
		workers = defaultWorkers
	}
	if workers > maxWorkers {
		workers = maxWorkers
	}

	nChunks := int((st.Size() + chunkSize - 1) / chunkSize)
	if workers > nChunks {
		workers = nChunks
	}
	hashes := make([]string, nChunks)
	var sent atomic.Int64
	report := func() {
		if opt.Progress != nil {
			opt.Progress(sent.Load(), st.Size())
		}
	}

	type job struct{ idx int }
	jobs := make(chan job, nChunks)
	for i := 0; i < nChunks; i++ {
		jobs <- job{idx: i}
	}
	close(jobs)

	var g errgroup.Group
	for w := 0; w < workers; w++ {
		g.Go(func() error {
			buf := make([]byte, chunkSize)
			for j := range jobs {
				off := int64(j.idx) * chunkSize
				n := chunkSize
				if rem := st.Size() - off; rem < n {
					n = rem
				}
				chunk := buf[:n]
				if _, err := f.ReadAt(chunk, off); err != nil {
					return fmt.Errorf("read chunk %d: %w", j.idx, err)
				}
				hash, err := c.uploadChunkWithRetry(chunk)
				if err != nil {
					return fmt.Errorf("chunk %d: %w", j.idx, err)
				}
				hashes[j.idx] = hash
				sent.Add(n)
				report()
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var out map[string]any
	err = c.DoJSON(http.MethodPost, "/api/upload/complete", map[string]any{
		"fileName":    fileName,
		"chunkHashes": hashes,
	}, &out)
	return out, err
}

func (c *Client) uploadChunkWithRetry(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	var last error
	for attempt := 1; attempt <= chunkAttempts; attempt++ {
		exists, err := c.ChunkExists(hash)
		if err != nil {
			last = err
		} else if exists {
			return hash, nil
		} else {
			got, _, err := c.PutChunk(data)
			if err == nil {
				if got != "" {
					return got, nil
				}
				return hash, nil
			}
			last = err
		}
		if attempt < chunkAttempts {
			time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
		}
	}
	if last == nil {
		last = fmt.Errorf("chunk upload failed")
	}
	return "", last
}

// CompleteUpload assembles previously uploaded chunk hashes.
func (c *Client) CompleteUpload(fileName string, hashes []string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/upload/complete", map[string]any{
		"fileName":    fileName,
		"chunkHashes": hashes,
	}, &out)
	return out, err
}

// HashBytes returns the SHA-256 hex of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
