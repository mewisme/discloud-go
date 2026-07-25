package store

// Role values for users.role.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Visibility values for files.visibility.
const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

// File status values for files.status (upload outcome badge).
const (
	FileStatusReady  = "ready"
	FileStatusReused = "reused"
)

// ShareMode values for files.share_mode.
const (
	ShareModeDownload = "download"
	ShareModeView     = "view"
)

// API token scopes.
const (
	ScopeUpload = "upload"
	ScopeRead   = "read"
	ScopeManage = "manage"
	ScopeAdmin  = "admin"
)

// Event kinds recorded for inspect analytics.
const (
	EventView     = "view"
	EventDownload = "download"
	EventRange    = "range"
)

// Upload session lifecycle statuses (not files.status).
const (
	UploadPending    = "pending"
	UploadUploading  = "uploading"
	UploadCompleting = "completing"
	UploadCompleted  = "completed"
	UploadCancelled  = "cancelled"
	UploadExpired    = "expired"
)
