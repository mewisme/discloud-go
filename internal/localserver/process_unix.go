//go:build !windows

package localserver

import (
	"errors"
	"syscall"
)

// detachAttr puts the server in its own session so it survives the CLI exit.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	// EPERM means the process exists but is owned by someone else.
	return err == nil || errors.Is(err, syscall.EPERM)
}

func terminate(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }

func forceKill(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }
