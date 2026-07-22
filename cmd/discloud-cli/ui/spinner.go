package ui

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner shows an indeterminate wait on stderr. Disabled when non-TTY or --json.
type spinner struct {
	mu     sync.Mutex
	w      io.Writer
	msg    string
	stopCh chan struct{}
	done   sync.WaitGroup
	closed bool
}

// ShowWaitUI reports whether spinner/progress UI should run (TTY and not JSON).
func ShowWaitUI() bool {
	return !JSON && IsTTY(os.Stderr)
}

func startSpinner(w io.Writer, msg string) *spinner {
	if w == nil {
		w = os.Stderr
	}
	s := &spinner{w: w, msg: msg, stopCh: make(chan struct{})}
	s.done.Add(1)
	go s.loop()
	return s
}

func (s *spinner) loop() {
	defer s.done.Done()
	t := time.NewTicker(80 * time.Millisecond)
	defer t.Stop()
	i := 0
	for {
		select {
		case <-s.stopCh:
			return
		case <-t.C:
			s.mu.Lock()
			if s.closed {
				s.mu.Unlock()
				return
			}
			on := false
			if f, ok := s.w.(*os.File); ok {
				on = ColorOn(f)
			}
			frame := Cyan(on, spinnerFrames[i%len(spinnerFrames)])
			fmt.Fprintf(s.w, "\r%s %s", frame, s.msg)
			i++
			s.mu.Unlock()
		}
	}
}

// Stop clears the spinner line.
func (s *spinner) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	close(s.stopCh)
	s.mu.Unlock()
	s.done.Wait()
	fmt.Fprint(s.w, "\r\033[K")
}

// WithSpinner runs fn while showing a wait spinner (TTY, non-JSON only).
func WithSpinner(msg string, fn func() error) error {
	if !ShowWaitUI() {
		return fn()
	}
	s := startSpinner(os.Stderr, msg)
	err := fn()
	s.Stop()
	return err
}

// WaitVal runs fn under a spinner and returns its value.
func WaitVal[T any](msg string, fn func() (T, error)) (T, error) {
	var v T
	err := WithSpinner(msg, func() error {
		var e error
		v, e = fn()
		return e
	})
	return v, err
}
