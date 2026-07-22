package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// IsTTY reports whether f is a terminal.
func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func writerColor(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return ColorOn(f)
	}
	return false
}

func canRewritePrompt(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && IsTTY(f)
}

// promptIcon writes the yellow "?" prefix used for all interactive prompts.
func promptIcon(w io.Writer) {
	fmt.Fprintf(w, "%s ", Yellow(writerColor(w), "?"))
}

// finishPrompt rewrites the previous prompt line: ✓ on success, ✗ on failure.
func finishPrompt(w io.Writer, ok bool, label, value string) {
	if !canRewritePrompt(w) {
		return
	}
	on := writerColor(w)
	icon := Green(on, IconOK)
	if !ok {
		icon = Red(on, IconFail)
	}
	fmt.Fprintf(w, "\033[1A\r\033[K%s %s%s\n", icon, label, value)
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

// fieldName turns "Username: " into "Username".
func fieldName(label string) string {
	name := strings.TrimSpace(label)
	name = strings.TrimRight(name, ":")
	name = strings.TrimSpace(name)
	if name == "" {
		return "value"
	}
	return name
}

// failPrompt rewrites the last prompt line to a clear error; returns errPrinted
// when rewrite worked so PrintError won't duplicate it.
func failPrompt(w io.Writer, msg string) error {
	err := errors.New(msg)
	if !canRewritePrompt(w) {
		return err
	}
	on := writerColor(w)
	fmt.Fprintf(w, "\033[1A\r\033[K%s %s\n", Red(on, IconFail), Bold(on, msg))
	return errPrinted{err}
}

// ReadPasswordPrompt reads a password from the TTY, echoing "*" per character.
func ReadPasswordPrompt(label string) (string, error) {
	promptIcon(os.Stderr)
	fmt.Fprint(os.Stderr, label)
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, state)
	pw, err := readPasswordMasked(os.Stdin, os.Stderr)
	if err != nil {
		return "", err
	}
	if pw == "" {
		return "", failPrompt(os.Stderr, "Password is required")
	}
	finishPrompt(os.Stderr, true, label, strings.Repeat("*", len(pw)))
	return pw, nil
}

// PromptDefault asks for a line with def pre-filled. Enter keeps def.
// On a TTY the value is editable in-place; otherwise empty input uses def.
func PromptDefault(label, def string) (string, error) {
	if JSON {
		return "", fmt.Errorf("%s required with --json", fieldName(label))
	}
	if !IsTTY(os.Stdin) {
		return "", fmt.Errorf("%s required", fieldName(label))
	}
	w := os.Stderr
	promptIcon(w)
	fmt.Fprint(w, label)
	fd := int(os.Stdin.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback: show default in brackets; empty line keeps it.
		on := writerColor(w)
		hint := ""
		if def != "" {
			hint = Dim(on, "["+def+"] ")
		}
		fmt.Fprint(w, hint)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		v := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
		if v == "" {
			v = def
		}
		finishPrompt(w, true, label, v)
		return v, nil
	}
	defer term.Restore(fd, state)
	fmt.Fprint(w, def)
	v, err := readLinePrefill(os.Stdin, w, []byte(def))
	if err != nil {
		return "", err
	}
	finishPrompt(w, true, label, v)
	return v, nil
}

// readLinePrefill edits a pre-filled buffer in raw mode (visible chars).
func readLinePrefill(r io.Reader, w io.Writer, buf []byte) (string, error) {
	var b [1]byte
	for {
		n, err := r.Read(b[:])
		if n > 0 {
			switch b[0] {
			case '\r', '\n':
				fmt.Fprint(w, "\r\n")
				return string(buf), nil
			case 127, '\b':
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					fmt.Fprint(w, "\b \b")
				}
			case 3: // Ctrl+C
				fmt.Fprint(w, "\r\n")
				return "", fmt.Errorf("interrupted")
			case 4: // Ctrl+D
				fmt.Fprint(w, "\r\n")
				return string(buf), nil
			default:
				if b[0] >= 32 && b[0] < 127 {
					buf = append(buf, b[0])
					fmt.Fprint(w, string(b[0]))
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

// ResolvePassword picks password from --password-stdin, deprecated positional, or TTY prompt.
// Never logs the value. Conflicting sources fail.
func ResolvePassword(positional string, passwordStdin bool) (string, error) {
	if positional != "" && passwordStdin {
		return "", fmt.Errorf("conflicting password inputs: positional password and --password-stdin")
	}
	if passwordStdin {
		return readPasswordStdin(os.Stdin)
	}
	if positional != "" {
		return positional, nil // deprecated; never echo
	}
	if !IsTTY(os.Stdin) {
		return "", fmt.Errorf("password required: use --password-stdin or run in a TTY")
	}
	pw, err := ReadPasswordPrompt("Password: ")
	if err != nil {
		return "", err
	}
	return pw, nil
}

// ResolveUsername returns username from args or TTY prompt.
func ResolveUsername(arg string) (string, error) {
	return ResolveArg(arg, "Username: ")
}

// ResolveArg returns arg if set; otherwise prompts on a TTY.
// Fails under --json or non-TTY when the value is missing.
func ResolveArg(arg, label string) (string, error) {
	if strings.TrimSpace(arg) != "" {
		return strings.TrimSpace(arg), nil
	}
	name := fieldName(label)
	if JSON {
		return "", fmt.Errorf("%s is required with --json", name)
	}
	if !IsTTY(os.Stdin) {
		return "", fmt.Errorf("%s is required", name)
	}
	v, err := promptLine(os.Stderr, os.Stdin, label)
	if err != nil {
		return "", err
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return "", failPrompt(os.Stderr, name+" is required")
	}
	finishPrompt(os.Stderr, true, label, v)
	return v, nil
}

// ArgOrEmpty returns args[i] or "" if out of range.
func ArgOrEmpty(args []string, i int) string {
	if i < 0 || i >= len(args) {
		return ""
	}
	return args[i]
}

// Confirm asks yes/no; default is No. Empty, EOF, and invalid input return false.
func Confirm(w io.Writer, r io.Reader, prompt string) bool {
	on := writerColor(w)
	label := prompt + Dim(on, " [y/N]") + " "
	promptIcon(w)
	fmt.Fprint(w, label)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return false
	}
	s := strings.TrimSpace(strings.ToLower(line))
	ok := s == "y" || s == "yes"
	answer := "No"
	if ok {
		answer = "Yes"
	}
	finishPrompt(w, true, label, answer)
	return ok
}

// pickIndex prints a numbered list to w and returns the chosen 0-based index.
func pickIndex(w io.Writer, r io.Reader, labels []string) (int, error) {
	if len(labels) == 0 {
		return -1, fmt.Errorf("nothing to select")
	}
	for i, label := range labels {
		fmt.Fprintf(w, "  %d) %s\n", i+1, label)
	}
	return pickNumber(w, r, len(labels))
}

// PickFile prints a file table then lets the user pick by ↑↓ or index.
// On success in a TTY, the table and prompt are cleared so the caller can
// replace them with the next screen (e.g. inspect).
func PickFile(w io.Writer, r io.Reader, files []File) (int, error) {
	if len(files) == 0 {
		return -1, fmt.Errorf("nothing to select")
	}
	tableLines := len(files) + 4 // rule + header + rule + rows + rule
	if err := printPickFileTable(w, files); err != nil {
		return -1, err
	}
	in, okIn := r.(*os.File)
	if okIn && IsTTY(in) && canRewritePrompt(w) {
		fd := int(in.Fd())
		state, err := term.MakeRaw(fd)
		if err == nil {
			defer term.Restore(fd, state)
			idx, err := pickFileRaw(w, in, len(files), func(i int) string {
				return files[i].Name
			})
			if err != nil {
				return -1, err
			}
			// Cursor is still on the prompt line — wipe table + prompt.
			clearLinesUp(w, tableLines)
			return idx, nil
		}
	}
	idx, err := pickNumber(w, r, len(files))
	if err != nil {
		return -1, err
	}
	// Cursor is one line below the finished prompt.
	clearLinesUp(w, tableLines+1)
	return idx, nil
}

func clearLinesUp(w io.Writer, up int) {
	if up <= 0 || !canRewritePrompt(w) {
		return
	}
	fmt.Fprintf(w, "\033[%dA\r\033[J", up)
}

// ClearLinesUp moves the cursor up and clears to end of screen (TTY only).
func ClearLinesUp(w io.Writer, up int) {
	clearLinesUp(w, up)
}

func pickNumber(w io.Writer, r io.Reader, n int) (int, error) {
	label := fmt.Sprintf("Select (1-%d): ", n)
	promptIcon(w)
	fmt.Fprint(w, label)
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil {
		return -1, fmt.Errorf("selection cancelled")
	}
	s := strings.TrimSpace(line)
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 || v > n {
		finishPrompt(w, false, label, s)
		return -1, fmt.Errorf("invalid selection")
	}
	finishPrompt(w, true, label, strconv.Itoa(v))
	return v - 1, nil
}

// pickFileRaw runs after stdin is already in raw mode.
// ↑↓ move selection, digits type an index, Enter confirms, Esc/Ctrl+C cancel.
// On success the cursor stays on the prompt line (no newline) so the caller
// can clear the pick UI in place.
func pickFileRaw(w io.Writer, in *os.File, n int, labelAt func(int) string) (int, error) {
	on := writerColor(w)
	hint := fmt.Sprintf("Select (1-%d, ↑↓): ", n)
	cur := 0
	typed := ""

	draw := func() {
		idx := cur
		if typed != "" {
			if v, err := strconv.Atoi(typed); err == nil && v >= 1 && v <= n {
				idx = v - 1
			}
		}
		cur = idx
		fmt.Fprintf(w, "\r\033[K%s %s%s  %s",
			Yellow(on, "?"), hint,
			Cyan(on, strconv.Itoa(idx+1)),
			Dim(on, labelAt(idx)))
	}
	draw()

	var b [1]byte
	for {
		_, err := in.Read(b[:])
		if err != nil {
			fmt.Fprint(w, "\r\n")
			return -1, fmt.Errorf("selection cancelled")
		}
		switch b[0] {
		case '\r', '\n':
			return cur, nil
		case 3: // Ctrl+C
			fmt.Fprint(w, "\r\n")
			return -1, fmt.Errorf("interrupted")
		case 27: // Esc / CSI
			var seq [2]byte
			if _, err := in.Read(seq[0:1]); err != nil || seq[0] != '[' {
				fmt.Fprint(w, "\r\n")
				return -1, fmt.Errorf("selection cancelled")
			}
			if _, err := in.Read(seq[1:2]); err != nil {
				fmt.Fprint(w, "\r\n")
				return -1, fmt.Errorf("selection cancelled")
			}
			typed = ""
			switch seq[1] {
			case 'A': // up
				if cur > 0 {
					cur--
				}
			case 'B': // down
				if cur < n-1 {
					cur++
				}
			}
			draw()
		case 127, '\b':
			if len(typed) > 0 {
				typed = typed[:len(typed)-1]
				draw()
			}
		default:
			if b[0] >= '0' && b[0] <= '9' {
				next := typed + string(b[0])
				if v, err := strconv.Atoi(next); err == nil && v <= n {
					typed = next
					if v >= 1 {
						cur = v - 1
					}
					draw()
				}
			}
		}
	}
}
