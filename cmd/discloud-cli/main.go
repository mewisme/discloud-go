// Command discloud-cli is the DisCloud API client.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mewisme/discloud-go/internal/client"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "discloud-cli: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return fmt.Errorf("missing command")
	}
	cfg := client.DefaultConfig()
	// Global flags before subcommand: --base --origin
	args = parseGlobal(args, &cfg)

	c, err := client.New(cfg)
	if err != nil {
		return err
	}

	switch args[0] {
	case "version", "--version", "-V":
		fmt.Println(version)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	case "config":
		return cmdConfig(c)
	case "auth":
		return cmdAuth(c, args[1:])
	case "upload":
		return cmdUpload(c, args[1:])
	case "upload-raw":
		return cmdUploadRaw(c, args[1:])
	case "chunks":
		return cmdChunks(c, args[1:])
	case "get":
		return cmdGet(c, args[1:])
	case "files":
		return cmdFiles(c, args[1:])
	case "info":
		return printJSON(c.GetInfo())
	case "health":
		s, err := c.Health()
		if err != nil {
			return err
		}
		fmt.Println(s)
		return nil
	case "ready":
		s, err := c.Ready()
		if err != nil {
			return err
		}
		fmt.Println(s)
		return nil
	default:
		return fmt.Errorf("unknown command %q (try: discloud-cli help)", args[0])
	}
}

func parseGlobal(args []string, cfg *client.Config) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--base" && i+1 < len(args):
			cfg.BaseURL = strings.TrimRight(args[i+1], "/")
			i++
		case strings.HasPrefix(a, "--base="):
			cfg.BaseURL = strings.TrimRight(strings.TrimPrefix(a, "--base="), "/")
		case a == "--origin" && i+1 < len(args):
			cfg.Origin = strings.TrimRight(args[i+1], "/")
			i++
		case strings.HasPrefix(a, "--origin="):
			cfg.Origin = strings.TrimRight(strings.TrimPrefix(a, "--origin="), "/")
		default:
			out = append(out, a)
		}
	}
	return out
}

func cmdConfig(c *client.Client) error {
	cfg := c.Config()
	fmt.Printf("base:   %s\n", cfg.BaseURL)
	fmt.Printf("origin: %s\n", cfg.Origin)
	fmt.Printf("cookies:%s\n", cfg.CookiePath)
	return nil
}

func cmdAuth(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: discloud-cli auth signup|signin|signout|me|password …")
	}
	switch args[0] {
	case "signup":
		if len(args) < 3 {
			return fmt.Errorf("usage: discloud-cli auth signup <username> <password>")
		}
		return printJSON(c.SignUp(args[1], args[2]))
	case "signin":
		if len(args) < 3 {
			return fmt.Errorf("usage: discloud-cli auth signin <username> <password>")
		}
		return printJSON(c.SignIn(args[1], args[2]))
	case "signout":
		return c.SignOut()
	case "me":
		return printJSON(c.Me())
	case "password":
		if len(args) < 3 {
			return fmt.Errorf("usage: discloud-cli auth password <current> <new>")
		}
		return c.ChangePassword(args[1], args[2])
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}
}

func cmdUpload(c *client.Client, args []string) error {
	path, name, workers, err := parseUploadFlags(args)
	if err != nil {
		return err
	}
	out, err := c.UploadChunked(path, client.UploadChunkedOptions{
		FileName: name,
		Workers:  workers,
		Progress: func(sent, total int64) {
			pct := 0.0
			if total > 0 {
				pct = float64(sent) * 100 / float64(total)
			}
			fmt.Fprintf(os.Stderr, "\rupload %s / %s (%.0f%%)", client.FormatBytes(sent), client.FormatBytes(total), pct)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr)
		return err
	}
	fmt.Fprintln(os.Stderr)
	return printJSONVal(out)
}

func cmdUploadRaw(c *client.Client, args []string) error {
	path, name, _, err := parseUploadFlags(args)
	if err != nil {
		return err
	}
	return printJSON(c.UploadRaw(path, name))
}

func parseUploadFlags(args []string) (path, name string, workers int, err error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--name" && i+1 < len(args):
			name = args[i+1]
			i++
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case a == "--workers" && i+1 < len(args):
			workers, err = strconv.Atoi(args[i+1])
			if err != nil {
				return "", "", 0, fmt.Errorf("--workers: %w", err)
			}
			i++
		case strings.HasPrefix(a, "--workers="):
			workers, err = strconv.Atoi(strings.TrimPrefix(a, "--workers="))
			if err != nil {
				return "", "", 0, fmt.Errorf("--workers: %w", err)
			}
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return "", "", 0, fmt.Errorf("usage: discloud-cli upload <path> [--name name] [--workers N]")
	}
	return rest[0], name, workers, nil
}

func cmdChunks(c *client.Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: discloud-cli chunks check|put …")
	}
	switch args[0] {
	case "check":
		if len(args) < 2 {
			return fmt.Errorf("usage: discloud-cli chunks check <sha256>")
		}
		ok, err := c.ChunkExists(args[1])
		if err != nil {
			return err
		}
		if ok {
			fmt.Println(`{"exists":true}`)
		} else {
			fmt.Println(`{"exists":false}`)
		}
		return nil
	case "put":
		if len(args) < 2 {
			return fmt.Errorf("usage: discloud-cli chunks put <path>")
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		hash, existed, err := c.PutChunk(data)
		if err != nil {
			return err
		}
		return printJSONVal(map[string]any{"hash": hash, "existed": existed})
	default:
		return fmt.Errorf("unknown chunks command %q", args[0])
	}
}

func cmdGet(c *client.Client, args []string) error {
	var (
		id, name, token, out string
		download, asJSON     bool
		rest                 []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--name" && i+1 < len(args):
			name = args[i+1]
			i++
		case strings.HasPrefix(a, "--name="):
			name = strings.TrimPrefix(a, "--name=")
		case a == "--token" && i+1 < len(args):
			token = args[i+1]
			i++
		case strings.HasPrefix(a, "--token="):
			token = strings.TrimPrefix(a, "--token=")
		case a == "--out" && i+1 < len(args):
			out = args[i+1]
			i++
		case strings.HasPrefix(a, "--out="):
			out = strings.TrimPrefix(a, "--out=")
		case a == "--download":
			download = true
		case a == "--json":
			asJSON = true
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return fmt.Errorf("usage: discloud-cli get <id> [--name name] [--download] [--json] [--token t] [--out path]")
	}
	id = rest[0]
	if out == "" && download && !asJSON {
		out = id
		if name != "" {
			out = filepath.Base(name)
		}
	}
	data, err := c.Download(id, client.DownloadOptions{
		Name: name, Download: download, JSON: asJSON, Token: token, OutPath: out,
	})
	if err != nil {
		return err
	}
	if out != "" {
		fmt.Fprintf(os.Stderr, "wrote %s\n", out)
		return nil
	}
	os.Stdout.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		fmt.Println()
	}
	return nil
}

func cmdFiles(c *client.Client, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: discloud-cli files list|get|inspect|visibility|rotate-token|delete …")
	}
	switch args[0] {
	case "list":
		limit, offset := 0, 0
		for i := 1; i < len(args); i++ {
			a := args[i]
			switch {
			case a == "--limit" && i+1 < len(args):
				limit, _ = strconv.Atoi(args[i+1])
				i++
			case strings.HasPrefix(a, "--limit="):
				limit, _ = strconv.Atoi(strings.TrimPrefix(a, "--limit="))
			case a == "--offset" && i+1 < len(args):
				offset, _ = strconv.Atoi(args[i+1])
				i++
			case strings.HasPrefix(a, "--offset="):
				offset, _ = strconv.Atoi(strings.TrimPrefix(a, "--offset="))
			}
		}
		return printJSON(c.ListFiles(limit, offset))
	case "get":
		id, token, err := idToken(args[1:])
		if err != nil {
			return err
		}
		return printJSON(c.GetFile(id, token))
	case "inspect":
		id, token, err := idToken(args[1:])
		if err != nil {
			return err
		}
		return printJSON(c.Inspect(id, token))
	case "visibility":
		if len(args) < 3 {
			return fmt.Errorf("usage: discloud-cli files visibility <id> public|private")
		}
		return printJSON(c.SetVisibility(args[1], args[2]))
	case "rotate-token":
		if len(args) < 2 {
			return fmt.Errorf("usage: discloud-cli files rotate-token <id>")
		}
		return printJSON(c.RotateToken(args[1]))
	case "delete":
		if len(args) < 2 {
			return fmt.Errorf("usage: discloud-cli files delete <id>")
		}
		return c.DeleteFile(args[1])
	default:
		return fmt.Errorf("unknown files command %q", args[0])
	}
}

func idToken(args []string) (id, token string, err error) {
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--token" && i+1 < len(args):
			token = args[i+1]
			i++
		case strings.HasPrefix(a, "--token="):
			token = strings.TrimPrefix(a, "--token=")
		default:
			rest = append(rest, a)
		}
	}
	if len(rest) != 1 {
		return "", "", fmt.Errorf("expected <id>")
	}
	return rest[0], token, nil
}

func printJSON(v any, err error) error {
	if err != nil {
		return err
	}
	return printJSONVal(v)
}

func printJSONVal(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `discloud %s — DisCloud API client

Usage:
  discloud [--base URL] [--origin URL] <command> …

Commands:
  auth signup|signin|signout|me|password
  upload <path> [--name name] [--workers N]   resumable chunked upload
  upload-raw <path> [--name name]             single POST /api/upload
  chunks check <sha256>
  chunks put <path>
  get <id> [--name name] [--download] [--json] [--token t] [--out path]
  files list [--limit n] [--offset n]
  files get|inspect <id> [--token t]
  files visibility <id> public|private
  files rotate-token <id>
  files delete <id>
  info | health | ready | config | version

Env:
  DISCLOUD_BASE     API origin
  DISCLOUD_ORIGIN   WEB_ORIGIN for CSRF
  Config file: ~/.config/discloud/config.json
`, version)
}
