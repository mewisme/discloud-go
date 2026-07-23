package server

import "time"

// Product retention / session constants (not env-configurable).
const (
	anonymousRetention     = 7 * 24 * time.Hour
	authenticatedRetention = 30 * 24 * time.Hour
	downloadExtension      = 7 * 24 * time.Hour
	maxRetentionFromNow    = 30 * 24 * time.Hour
	sessionTTL             = 30 * 24 * time.Hour
	cleanupInterval        = time.Hour
	cleanupBatchSize       = 100

	uploadSessionAnonTTL  = 24 * time.Hour
	uploadSessionAuthTTL  = 48 * time.Hour
	maxOpenUploadSessions = 20
	maxPathDepth          = 32
	maxFileNameLen        = 512

	sessionCookieName = "discloud_session"
	uploadTokenHeader = "X-Upload-Token"

	authRateLimit       = 10
	authRateLimitWindow = 15 * time.Minute
)
