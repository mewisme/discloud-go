package main

import (
	"encoding/json"
	"fmt"
	"time"
)

// pickerSafetyLimit caps how many files interactive selection will load.
const pickerSafetyLimit = 500

// pageSize is the page size used when fetching for the picker (server max 200).
const pickerPageSize = 100

// FileItem is a file metadata / links payload from the API.
type FileItem struct {
	FileID             string    `json:"fileId"`
	FileName           string    `json:"fileName"`
	FileSize           int64     `json:"fileSize"`
	ChunkSize          int64     `json:"chunkSize,omitempty"`
	Visibility         string    `json:"visibility"`
	Status             string    `json:"status,omitempty"`
	OwnedByCurrentUser bool      `json:"ownedByCurrentUser,omitempty"`
	CreatedAt          time.Time `json:"createdAt"`
	ExpiresAt          time.Time `json:"expiresAt"`
	PasswordProtected  bool      `json:"passwordProtected,omitempty"`
	ShareMode          string    `json:"shareMode,omitempty"`
	MaxDownloads       *int      `json:"maxDownloads"`
	DownloadCount      int       `json:"downloadCount,omitempty"`
	URL                string    `json:"url,omitempty"`
	LongURL            string    `json:"longURL,omitempty"`
	DownloadURL        string    `json:"downloadURL,omitempty"`
	LongDownloadURL    string    `json:"longDownloadURL,omitempty"`
	AccessToken        string    `json:"accessToken,omitempty"`
}

// FilesList is GET /api/files.
type FilesList struct {
	Files []FileItem `json:"files"`
}

// AuthUser is signup/signin response.
type AuthUser struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	Role              string    `json:"role,omitempty"`
	CreatedAt         time.Time `json:"createdAt,omitempty"`
	PasswordChangedAt time.Time `json:"passwordChangedAt,omitempty"`
	DefaultVisibility string    `json:"defaultVisibility,omitempty"`
}

// MeStats is the stats object inside MeResponse.
type MeStats struct {
	FileCount         int64 `json:"fileCount"`
	TotalBytes        int64 `json:"totalBytes"`
	PublicCount       int64 `json:"publicCount"`
	PrivateCount      int64 `json:"privateCount"`
	ExpiringSoonCount int64 `json:"expiringSoonCount"`
}

// MeSession is the session object inside MeResponse.
type MeSession struct {
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"userAgent,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
}

// MeResponse is GET /api/auth/me.
type MeResponse struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	Role              string    `json:"role,omitempty"`
	CreatedAt         time.Time `json:"createdAt,omitempty"`
	PasswordChangedAt time.Time `json:"passwordChangedAt,omitempty"`
	Stats             MeStats   `json:"stats"`
	Session           MeSession `json:"session"`
	Preferences       struct {
		DefaultVisibility string `json:"defaultVisibility"`
	} `json:"preferences"`
}

// InspectResponse is GET /api/files/{id}/inspect.
type InspectResponse struct {
	FileID             string     `json:"fileId"`
	FileName           string     `json:"fileName"`
	FileSize           int64      `json:"fileSize"`
	ChunkSize          int64      `json:"chunkSize"`
	ChunkCount         int        `json:"chunkCount"`
	CreatedAt          time.Time  `json:"createdAt"`
	ExpiresAt          time.Time  `json:"expiresAt"`
	Visibility         string     `json:"visibility"`
	Status             string     `json:"status,omitempty"`
	OwnedByCurrentUser bool       `json:"ownedByCurrentUser"`
	PasswordProtected  bool       `json:"passwordProtected,omitempty"`
	ShareMode          string     `json:"shareMode,omitempty"`
	MaxDownloads       *int       `json:"maxDownloads"`
	DownloadCount      int        `json:"downloadCount,omitempty"`
	Views              int64      `json:"views"`
	Downloads          int64      `json:"downloads"`
	Ranges             int64      `json:"ranges"`
	BytesServed        int64      `json:"bytesServed"`
	UniqueVisitors     int64      `json:"uniqueVisitors"`
	LastAccessAt       *time.Time `json:"lastAccessAt"`
	URL                string     `json:"url"`
	LongURL            string     `json:"longURL"`
	DownloadURL        string     `json:"downloadURL"`
	LongDownloadURL    string     `json:"longDownloadURL"`
}

// ChunkPutResult is chunks put output.
type ChunkPutResult struct {
	Hash    string `json:"hash"`
	Existed bool   `json:"existed"`
}

// ChunkExistsResult is chunks check output.
type ChunkExistsResult struct {
	Exists bool `json:"exists"`
}

// decode re-marshals a client map/any payload into a typed value.
func decode[T any](v any) (T, error) {
	var out T
	b, err := json.Marshal(v)
	if err != nil {
		return out, fmt.Errorf("encode response: %w", err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}
