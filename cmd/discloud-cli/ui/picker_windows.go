//go:build windows

package ui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const vkReturn = 0x0D

// INPUT_RECORD / KEY_EVENT_RECORD layout (Unicode). sizeof == 20 on Windows.
type consoleInputRecord struct {
	EventType uint16
	_         uint16
	KeyDown   int32
	Repeat    uint16
	VK        uint16
	Scan      uint16
	Char      uint16
	Control   uint32
}

var (
	modKernel32           = windows.NewLazySystemDLL("kernel32.dll")
	procPeekConsoleInputW = modKernel32.NewProc("PeekConsoleInputW")
	procReadConsoleInputW = modKernel32.NewProc("ReadConsoleInputW")
)

func openConsoleInput() (windows.Handle, func(), error) {
	// CONIN$ is the real console even when STD_INPUT_HANDLE is a pipe/ConPTY quirk.
	h, err := windows.CreateFile(
		windows.StringToUTF16Ptr("CONIN$"),
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		0,
		0,
	)
	if err == nil && h != 0 && h != windows.InvalidHandle {
		return h, func() { _ = windows.CloseHandle(h) }, nil
	}
	h, err = windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil || h == 0 || h == windows.InvalidHandle {
		return 0, func() {}, err
	}
	return h, func() {}, nil
}

// pickerEnterPressed is non-blocking: peeks console KEY_EVENT for Enter.
// Never calls a blocking Read — that stuck the countdown on Windows.
func pickerEnterPressed() bool {
	h, closeH, err := openConsoleInput()
	if err != nil {
		return false
	}
	defer closeH()

	// Drop mouse/window noise so KEY_EVENTs stay visible; restore after.
	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err == nil {
		quiet := mode &^ (windows.ENABLE_MOUSE_INPUT | windows.ENABLE_WINDOW_INPUT)
		_ = windows.SetConsoleMode(h, quiet)
		defer windows.SetConsoleMode(h, mode)
	}

	var n uint32
	if err := windows.GetNumberOfConsoleInputEvents(h, &n); err != nil || n == 0 {
		return false
	}
	if n > 64 {
		n = 64
	}
	recs := make([]consoleInputRecord, n)
	var read uint32
	if err := peekConsoleInput(h, &recs[0], n, &read); err != nil || read == 0 {
		return false
	}
	for _, r := range recs[:read] {
		if isEnterKey(r) {
			_ = windows.FlushConsoleInputBuffer(h)
			return true
		}
	}
	// Drain mouse/focus noise so the buffer does not grow forever.
	drainConsoleInput(h, read)
	return false
}

func isEnterKey(r consoleInputRecord) bool {
	if r.EventType != windows.KEY_EVENT {
		return false
	}
	if r.Char == '\r' || r.Char == '\n' {
		return true
	}
	return r.KeyDown != 0 && r.VK == vkReturn
}

func peekConsoleInput(h windows.Handle, buf *consoleInputRecord, length uint32, read *uint32) error {
	r1, _, e := procPeekConsoleInputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(buf)),
		uintptr(length),
		uintptr(unsafe.Pointer(read)),
	)
	if r1 == 0 {
		return e
	}
	return nil
}

func drainConsoleInput(h windows.Handle, n uint32) {
	if n == 0 {
		return
	}
	recs := make([]consoleInputRecord, n)
	var read uint32
	_, _, _ = procReadConsoleInputW.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&recs[0])),
		uintptr(n),
		uintptr(unsafe.Pointer(&read)),
	)
}
