//go:build !windows

package ui

import "context"

func startConsoleInterruptWatch(ctx context.Context, cancel context.CancelFunc) func() {
	return func() {}
}
