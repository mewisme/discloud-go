package store

import "errors"

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("file not found")

// ErrConflict is returned for unique constraint violations (e.g. username).
var ErrConflict = errors.New("conflict")

// ErrPartConflict is returned when a session part is re-registered with a different hash.
var ErrPartConflict = errors.New("upload part hash conflict")

// ErrUploadNotActive is returned when mutating a non-open upload session.
var ErrUploadNotActive = errors.New("upload session not active")

// ErrDownloadLimit is returned when a share download cap is exhausted.
var ErrDownloadLimit = errors.New("download limit reached")

// ErrNotSupported is returned when a backend does not implement a feature
// (e.g. SQLite has no runtime config table).
var ErrNotSupported = errors.New("not supported")
