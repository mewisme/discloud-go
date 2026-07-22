package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

func isTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

// promptIcon writes the yellow "?" prefix used for all interactive prompts.
func promptIcon(w io.Writer) {
	on := false
	if f, ok := w.(*os.File); ok {
		on = colorOn(f)
	}
	fmt.Fprintf(w, "%s ", yellow(on, "?"))
}

func promptLine(w io.Writer, r io.Reader, label string) (string, error) {
	promptIcon(w)
	fmt.Fprint(w, label)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// readPasswordPrompt reads a password from the TTY, echoing "*" per character.
func readPasswordPrompt(label string) (string, error) {
	promptIcon(os.Stderr)
	fmt.Fprint(os.Stderr, label)
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state)
	return readPasswordMasked(os.Stdin, os.Stderr)
}

// readPasswordMasked reads byte-by-byte, echoes "*", supports backspace.
// Enter submits; Ctrl+C aborts. Used after the terminal is in raw mode.
func readPasswordMasked(r io.Reader, w io.Writer) (string, error) {
	var buf []byte
	var b [1]byte
	for {
		n, err := r.Read(b[:])
		if n > 0 {
			switch b[0] {
			case '\r', '\n':
				fmt.Fprint(w, "\r\n")
				return string(buf), nil
			case 127, '\b': // backspace / delete
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					fmt.Fprint(w, "\b \b")
				}
			case 3: // Ctrl+C
				fmt.Fprint(w, "\r\n")
				return "", fmt.Errorf("interrupted")
			case 4: // Ctrl+D
				fmt.Fprint(w, "\r\n")
				if len(buf) == 0 {
					return "", fmt.Errorf("empty password")
				}
				return string(buf), nil
			default:
				if b[0] >= 32 && b[0] < 127 {
					buf = append(buf, b[0])
					fmt.Fprint(w, "*")
				}
			}
		}
		if err == io.EOF {
			fmt.Fprint(w, "\r\n")
			return string(buf), nil
		}
		if err != nil {
			return "", err
		}
	}
}

// readPasswordStdin reads one line from r, strips only the line ending, rejects empty.
func readPasswordStdin(r io.Reader) (string, error) {
	br := bufio.NewReader(r)
	line, err := br.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if len(line) == 0 && err == io.EOF {
		return "", fmt.Errorf("empty password from stdin")
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	if line == "" {
		return "", fmt.Errorf("empty password from stdin")
	}
	return line, nil
}

// resolvePassword picks password from --password-stdin, deprecated positional, or TTY prompt.
// Never logs the value. Conflicting sources fail.
func resolvePassword(positional string, passwordStdin bool) (string, error) {
	if positional != "" && passwordStdin {
		return "", fmt.Errorf("conflicting password inputs: positional password and --password-stdin")
	}
	if passwordStdin {
		return readPasswordStdin(os.Stdin)
	}
	if positional != "" {
		return positional, nil // deprecated; never echo
	}
	if !isTTY(os.Stdin) {
		return "", fmt.Errorf("password required: use --password-stdin or run in a TTY")
	}
	pw, err := readPasswordPrompt("Password: ")
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", fmt.Errorf("empty password")
	}
	return pw, nil
}

// resolveUsername returns username from args or TTY prompt.
func resolveUsername(arg string) (string, error) {
	if arg != "" {
		return arg, nil
	}
	if !isTTY(os.Stdin) {
		return "", fmt.Errorf("username required")
	}
	u, err := promptLine(os.Stderr, os.Stdin, "Username: ")
	if err != nil {
		return "", err
	}
	if u == "" {
		return "", fmt.Errorf("empty username")
	}
	return u, nil
}

// confirm asks yes/no; default is No. Empty, EOF, and invalid input return false.
func confirm(w io.Writer, r io.Reader, prompt string) bool {
	on := false
	if f, ok := w.(*os.File); ok {
		on = colorOn(f)
	}
	promptIcon(w)
	fmt.Fprintf(w, "%s%s ", prompt, dim(on, " [y/N]"))
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return false
	}
	s := strings.TrimSpace(strings.ToLower(line))
	return s == "y" || s == "yes"
}

// pickIndex prints a numbered list to w and returns the chosen 0-based index.
func pickIndex(w io.Writer, r io.Reader, labels []string) (int, error) {
	if len(labels) == 0 {
		return -1, fmt.Errorf("nothing to select")
	}
	for i, label := range labels {
		fmt.Fprintf(w, "  %d) %s\n", i+1, label)
	}
	promptIcon(w)
	fmt.Fprintf(w, "Select (1-%d): ", len(labels))
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("selection cancelled")
	}
	s := strings.TrimSpace(line)
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(labels) {
		return -1, fmt.Errorf("invalid selection")
	}
	return n - 1, nil
}
