# CLI

The released client command is **`discloud`** (Scoop/Homebrew shim). The built binary is named `discloud-cli`.

## Install

From a running DisCloud instance (proxied via the web app or API):

```bash
curl -fsSL https://files.example.com/install.sh | sh   # macOS / Linux
irm https://files.example.com/install.ps1 | iex        # Windows
```

Package managers / from source:

```bash
scoop install mew/discloud-cli
brew install --cask discloud-cli
go build -o discloud-cli ./cmd/discloud-cli
```

## Global flags

| Flag | Env | Purpose |
| --- | --- | --- |
| `--base` | `DISCLOUD_BASE` | API origin |
| `--origin` | `DISCLOUD_ORIGIN` | Must match server `DISCLOUD_CORS` for cookie CSRF |
| `--json` | — | Emit one JSON document on stdout |

Config resolution: env → `config.json` → localhost defaults. Auth: **session cookie jar wins over** `DISCLOUD_TOKEN` / config token.

Config directory:

- Linux/macOS: `$XDG_CONFIG_HOME/discloud` or `~/.config/discloud`
- Windows: `%APPDATA%\discloud`

Contains `config.json`, `cookies`, and upload checkpoints under `uploads/`.

## Command tree

```text
discloud
├── auth
│   ├── login|signin [username] [password]   (--password-stdin)
│   ├── logout|signout
│   ├── signup [username] [password]         (--password-stdin)
│   ├── me
│   ├── password [current] [new]
│   └── token
│       ├── create --name --scopes [--expires]
│       ├── list
│       ├── info|validate|whoami [--prompt]
│       └── revoke <id> [-y]
├── upload [path...]                         (--abort, --name, --workers)
│   └── pick                                 (zenity multi-file dialog)
├── chunks
│   ├── check [sha256]
│   └── put [path]
├── get [id]                                 (--download, --out, --password, --token, --meta, --name)
├── files
│   ├── list [--limit] [--offset]
│   ├── get [id]
│   ├── inspect [id]
│   ├── verify <path> <sha256>
│   ├── visibility [id] {public|private} [-y]
│   ├── share [id]                           (--password, --mode, --max-downloads, …)
│   ├── revoke [id] [-y]
│   ├── rotate-token [id] [-y]
│   └── delete [id] [-y]
├── admin
│   └── overview
├── health
├── ready
├── config
│   ├── set [--base] [--origin] [token|--token]
│   ├── unset …
│   └── path
└── version
```

Run `discloud <command> --help` for exact flags.

## Interactive behavior

- Missing args often prompt (username, password, file id picker).
- Passwords prefer secure prompts or `--password-stdin` over positional args.
- TTY-aware tables, spinners, and upload progress bars.
- File/folder paths: pass args, or omit to prompt; `upload pick` opens a GUI multi-select when available.
- `--json` disables interactive prompts that would mix with machine output.
- User abort (Ctrl+C / Esc / dialog cancel) exits quietly (status 0).

## Upload checkpoints and resume

- Checkpoints live under the config dir `uploads/`.
- Resume matches **absolute path + size + mtime** (and chunk size).
- `discloud upload --abort <path>` cancels the server upload session for that checkpoint and clears local state.
- Mid-upload cancel (Ctrl+C / Esc) stops workers; the session remains resumable until cancelled or expired.

## Examples

```bash
discloud config set --base https://api.example.com --origin https://files.example.com
discloud auth login
discloud upload ./file.bin
discloud upload ./my-folder
discloud upload --abort ./file.bin
discloud files list
discloud files share <file-id> --password 'secret-pass' --mode view --max-downloads 10
discloud files revoke <file-id> -y
discloud get <file-id> --password 'secret-pass' --download --out ./downloaded.bin
discloud files verify ./downloaded.bin <sha256>

# API tokens
discloud auth token create --name ci --scopes upload,read,manage
discloud config set --token   # prompts for dc_…
discloud auth token info
discloud auth token revoke <id> -y
```

Scopes: `upload` | `read` | `manage` | `admin` (`admin` also needs the admin role). Raw token is shown once at create.

Admin overview (cookie as admin, or PAT with `admin` scope):

```bash
discloud admin overview
```
