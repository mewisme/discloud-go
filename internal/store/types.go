// Package store defines backend-neutral persistence models, errors, and
// constants. Concrete backends (e.g. internal/store/pgx) implement the
// server.Store interface and return these types.
package store

import "time"

// FilePart is one Discord attachment that makes up a stored file.
type FilePart struct {
	MessageID string
	BotID     int // token slot; -1 if unknown (legacy)
}

// File is a stored file: its metadata plus the ordered Discord parts.
type File struct {
	ID                   string     `json:"fileId"`
	Name                 string     `json:"fileName"`
	Size                 int64      `json:"fileSize"`
	ChunkSize            int64      `json:"chunkSize"`
	CreatedAt            time.Time  `json:"createdAt"`
	OwnerUserID          *string    `json:"ownerUserId,omitempty"`
	Visibility           string     `json:"visibility"`
	Status               string     `json:"status"`
	AccessTokenHash      string     `json:"-"`
	AccessTokenRotatedAt *time.Time `json:"-"`
	ExpiresAt            time.Time  `json:"expiresAt"`
	PasswordHash         string     `json:"-"`
	MaxDownloads         *int       `json:"maxDownloads,omitempty"`
	DownloadCount        int        `json:"downloadCount"`
	ShareMode            string     `json:"shareMode"`
	SHA256               string     `json:"sha256,omitempty"` // discloud-sha256-v1; empty for legacy
	Parts                []FilePart `json:"-"`
}

// Chunk is one entry in the content-addressed chunk store.
type Chunk struct {
	Hash      string
	MessageID string
	BotID     int // token slot; -1 if unknown
	Size      int64
}

// User is an account row.
type User struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	PasswordHash      string    `json:"-"`
	Role              string    `json:"role"`
	DefaultVisibility string    `json:"defaultVisibility"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
	PasswordChangedAt time.Time `json:"passwordChangedAt"`
}

// Session is a login session; TokenHash is never JSON-encoded by callers.
type Session struct {
	ID         string
	UserID     string
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
	UserAgent  string
	IP         string
	LastSeenAt time.Time
}

// OwnerStats summarizes a user's files (counts, bytes, expiring-soon).
type OwnerStats struct {
	FileCount         int64
	TotalBytes        int64
	PublicCount       int64
	PrivateCount      int64
	ExpiringSoonCount int64
}

// UploadSession is a resumable upload ledger (bytes live in chunk_store).
type UploadSession struct {
	ID                string
	OwnerUserID       *string
	ResumeTokenHash   string
	FileName          string
	FileSize          int64
	ChunkSize         int
	ChunkCount        int
	Status            string
	FileID            *string
	ClientFingerprint string
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ExpiresAt         time.Time
	Parts             map[int]string // idx → chunk hash (registered only)
}

// APIToken is a personal access token row (hash only; raw shown once at create).
type APIToken struct {
	ID         string
	UserID     string
	Name       string
	TokenHash  string
	Scopes     []string
	ExpiresAt  *time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// FileSharePatch is a partial update for share settings. Nil fields are unchanged.
// ClearPassword / ClearMaxDownloads force-clear even when the companion pointer is nil.
type FileSharePatch struct {
	PasswordHash      *string // set hash; use ClearPassword to remove
	ClearPassword     bool
	ExpiresAt         *time.Time // set absolute expiry
	MaxDownloads      *int       // set cap
	ClearMaxDownloads bool       // unlimited
	ShareMode         *string
}

// Event is one access to a file.
type Event struct {
	FileID      string
	Kind        string
	Bytes       int64
	VisitorHash string
}

// FileInspect is the inspect payload (without share URLs — server adds those).
type FileInspect struct {
	File
	Views          int64      `json:"views"`
	Downloads      int64      `json:"downloads"`
	Ranges         int64      `json:"ranges"`
	BytesServed    int64      `json:"bytesServed"`
	UniqueVisitors int64      `json:"uniqueVisitors"`
	LastAccessAt   *time.Time `json:"lastAccessAt,omitempty"`
	ChunkCount     int        `json:"chunkCount"`
}

// AdminOverview is instance-wide ops aggregates for GET /api/admin/overview.
type AdminOverview struct {
	FileCount       int64
	TotalBytes      int64
	ChunkStoreCount int64
	UserCount       int64
	AdminCount      int64
	OpenSessions    int64
	Completed24h    int64
	Expired24h      int64
	Cancelled24h    int64
	Downloads       int64 // lifetime SUM(files.downloads)
	BytesServed     int64 // lifetime SUM(files.bytes_served)
}
