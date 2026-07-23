package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
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
// Uses server upload sessions when advertised by GET /api/info; otherwise legacy complete.
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
	useSession := info.Uploads != nil && info.Uploads.Sessions

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

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	size, mtime := fileFingerprint(st)

	var uploadID, resumeToken string
	registered := map[int]string{}
	if useSession {
		if cp, err := loadCheckpoint(path, size, mtime); err == nil && cp.ChunkSize == chunkSize {
			uploadID, resumeToken = cp.UploadID, cp.ResumeToken
			for k, v := range cp.Hashes {
				registered[k] = v
			}
			var prog uploadProgress
			if err := c.DoJSONUploadToken(http.MethodGet, "/api/uploads/"+uploadID, nil, &prog, resumeToken); err != nil {
				uploadID, resumeToken = "", ""
				registered = map[int]string{}
			} else if prog.Status == "cancelled" || prog.Status == "expired" {
				uploadID, resumeToken = "", ""
				registered = map[int]string{}
			} else {
				for _, idx := range prog.MissingIndices {
					delete(registered, idx)
				}
			}
		}
		if uploadID == "" {
			var created struct {
				UploadID    string `json:"uploadId"`
				ResumeToken string `json:"resumeToken"`
			}
			err := c.DoJSON(http.MethodPost, "/api/uploads", map[string]any{
				"fileName":          fileName,
				"fileSize":          st.Size(),
				"clientFingerprint": fmt.Sprintf("cli:%s", filepath.Base(abs)),
			}, &created)
			if err != nil {
				return nil, err
			}
			uploadID, resumeToken = created.UploadID, created.ResumeToken
		}
		_ = saveCheckpoint(&uploadCheckpoint{
			UploadID: uploadID, ResumeToken: resumeToken, Path: abs,
			Size: size, ModTimeUnix: mtime, ChunkSize: chunkSize, FileName: fileName, Hashes: registered,
		})
	}

	hashes := make([]string, nChunks)
	var sent atomic.Int64
	// Count already-registered as sent for progress.
	for idx, h := range registered {
		if idx >= 0 && idx < nChunks {
			hashes[idx] = h
			off := int64(idx) * chunkSize
			n := chunkSize
			if rem := st.Size() - off; rem < n {
				n = rem
			}
			sent.Add(n)
		}
	}
	report := func() {
		if opt.Progress != nil {
			opt.Progress(sent.Load(), st.Size())
		}
	}
	report()

	type job struct{ idx int }
	jobs := make(chan job, nChunks)
	for i := 0; i < nChunks; i++ {
		if hashes[i] != "" {
			continue
		}
		jobs <- job{idx: i}
	}
	close(jobs)

	var mu sync.Mutex
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
				if useSession {
					if err := c.DoJSONUploadToken(http.MethodPut,
						fmt.Sprintf("/api/uploads/%s/parts/%d", uploadID, j.idx),
						map[string]string{"hash": hash}, nil, resumeToken); err != nil {
						return fmt.Errorf("register part %d: %w", j.idx, err)
					}
					mu.Lock()
					registered[j.idx] = hash
					_ = saveCheckpoint(&uploadCheckpoint{
						UploadID: uploadID, ResumeToken: resumeToken, Path: abs,
						Size: size, ModTimeUnix: mtime, ChunkSize: chunkSize, FileName: fileName, Hashes: copyHashMap(registered),
					})
					mu.Unlock()
				}
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
	if useSession {
		err = c.DoJSONUploadToken(http.MethodPost, "/api/uploads/"+uploadID+"/complete", nil, &out, resumeToken)
		if err == nil {
			clearCheckpoint(path, size, mtime)
		}
		return out, err
	}
	err = c.DoJSON(http.MethodPost, "/api/upload/complete", map[string]any{
		"fileName":    fileName,
		"chunkHashes": hashes,
	}, &out)
	return out, err
}

type uploadProgress struct {
	Status         string `json:"status"`
	MissingIndices []int  `json:"missingIndices"`
}

func copyHashMap(in map[int]string) map[int]string {
	out := make(map[int]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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

// CompleteUpload assembles previously uploaded chunk hashes (legacy).
func (c *Client) CompleteUpload(fileName string, hashes []string) (map[string]any, error) {
	var out map[string]any
	err := c.DoJSON(http.MethodPost, "/api/upload/complete", map[string]any{
		"fileName":    fileName,
		"chunkHashes": hashes,
	}, &out)
	return out, err
}

// AbortUpload cancels the server session for a local checkpoint matching path.
func (c *Client) AbortUpload(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	size, mtime := fileFingerprint(st)
	cp, err := loadCheckpoint(path, size, mtime)
	if err != nil {
		return fmt.Errorf("no checkpoint for %s", path)
	}
	res, err := c.doWithHeaders(http.MethodDelete, "/api/uploads/"+cp.UploadID, nil, "", map[string]string{
		"X-Upload-Token": cp.ResumeToken,
	})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if err := decodeResponse(res, nil); err != nil {
		return err
	}
	clearCheckpoint(path, size, mtime)
	return nil
}

// HashBytes returns the SHA-256 hex of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
