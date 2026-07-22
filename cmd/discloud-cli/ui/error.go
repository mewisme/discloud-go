package ui

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/mewisme/discloud-go/internal/client"
)

// PrintError writes a clear, colored error to stderr (no "discloud-cli:" noise).
func PrintError(err error) {
	if err == nil || isErrPrinted(err) {
		return
	}
	on := ColorOn(os.Stderr)
	title, detail, hint := describeError(err)
	fmt.Fprintf(os.Stderr, "%s %s\n", Red(on, IconFail), Bold(on, title))
	if detail != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", Dim(on, detail))
	}
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  %s %s\n", Cyan(on, IconInfo), hint)
	}
}

// errPrinted means the UI already showed the failure (prompt rewrite); skip a second line.
type errPrinted struct{ error }

func (e errPrinted) Error() string { return e.error.Error() }
func (e errPrinted) Unwrap() error { return e.error }

func isErrPrinted(err error) bool {
	var e errPrinted
	return errors.As(err, &e)
}

func describeError(err error) (title, detail, hint string) {
	var api *client.Error
	if errors.As(err, &api) {
		title = api.Message
		if title == "" {
			title = "Request failed"
		}
		detail = fmt.Sprintf("HTTP %d", api.Status)
		switch api.Status {
		case 401:
			hint = "sign in with: discloud auth login"
		case 403:
			hint = "you may not have permission for this resource"
		case 404:
			hint = "check the id or path"
		case 429:
			hint = "wait a moment and retry"
		case 502, 503, 504:
			hint = "API is up but unhealthy — try again shortly"
		}
		return title, detail, hint
	}

	if u := urlError(err); u != nil {
		host := u.URL
		if host == "" {
			host = u.Op
		}
		switch {
		case isConnRefused(err):
			return "Cannot reach API",
				host,
				"is the server running? check --base or: discloud config"
		case isTimeout(err):
			return "API timed out",
				host,
				"check network or --base"
		case isDNSError(err):
			return "Cannot resolve API host",
				host,
				"check the hostname in --base or config"
		default:
			msg := u.Err.Error()
			if msg == "" {
				msg = err.Error()
			}
			return "Request failed", host + " — " + msg, "check --base or: discloud config"
		}
	}

	msg := strings.TrimSpace(err.Error())
	// Drop redundant "discloud-cli:" if something wrapped it.
	msg = strings.TrimPrefix(msg, "discloud-cli: ")
	return msg, "", ""
}

func urlError(err error) *url.Error {
	var u *url.Error
	if errors.As(err, &u) {
		return u
	}
	return nil
}

func isConnRefused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var op *net.OpError
	if errors.As(err, &op) {
		if errors.Is(op.Err, syscall.ECONNREFUSED) {
			return true
		}
		low := strings.ToLower(op.Err.Error())
		if strings.Contains(low, "refused") || strings.Contains(low, "actively refused") {
			return true
		}
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "connection refused") || strings.Contains(s, "actively refused")
}

func isTimeout(err error) bool {
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "timeout")
}

func isDNSError(err error) bool {
	var d *net.DNSError
	if errors.As(err, &d) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "no such host") || strings.Contains(s, "server misbehaving")
}
