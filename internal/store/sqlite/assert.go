package sqlite

import "github.com/mewisme/discloud-go/internal/server"

// var _ server.Store = (*Store)(nil) fails to compile if Store drifts from
// the interface the HTTP server depends on.
var _ server.Store = (*Store)(nil)
