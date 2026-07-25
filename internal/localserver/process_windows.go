//go:build windows

package localserver

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code Windows reports for a live process.
const stillActive = 259

// detachAttr gives the server its own process group and no console so it
// survives the CLI exit.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
		HideWindow:    true,
	}
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// ponytail: Windows stop is a hard TerminateProcess — a detached process has no
// console, so CTRL_BREAK cannot reach it and the server never runs its graceful
// shutdown. SQLite/HTTP state survives because writes are transactional.
// Upgrade path: a job object plus an authenticated /shutdown endpoint.
func terminate(pid int) error { return forceKill(pid) }

func forceKill(pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.TerminateProcess(h, 1)
}
