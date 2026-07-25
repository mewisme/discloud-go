# CLI

The released client command is **`discloud`** (Scoop/Homebrew shim). The built binary is named `discloud-cli`.

## Install

From a running DisCloud instance (proxied via the web app or API):

```bash
curl -fsSL http://localhost:3000/install.sh | sh   # macOS / Linux
irm http://localhost:3000/install.ps1 | iex        # Windows
```

Public installer (or use):

```bash
curl -fsSL https://get.mewis.me/discloud.sh | sh       # macOS / Linux
irm https://get.mewis.me/discloud.ps1 | iex            # Windows
```

Root-repo scripts (`install.sh` / `install.ps1`) default `base`/`origin` to `http://localhost:8080` and `http://localhost:3000` (override with `DISCLOUD_BASE` / `DISCLOUD_ORIGIN`).

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

Config resolution: env → `config.yaml` → localhost defaults. Legacy `config.json` is migrated automatically to `config.yaml` (archived as `config.json.migrated`). Auth: **session cookie jar wins over** `DISCLOUD_TOKEN` / config token.

Config directory:

- Linux/macOS: `$XDG_CONFIG_HOME/discloud` or `~/.config/discloud`
- Windows: `%APPDATA%\discloud`

Contains `config.yaml`, `cookies`, and upload checkpoints under `uploads/`.

## Command tree

```text
discloud                                    (--base, --origin, --json, -v/--version)
├── auth
│   ├── login|signin [username] [password]  (--password-stdin)
│   ├── logout|signout
│   ├── signup [username] [password]        (--password-stdin)
│   ├── me
│   ├── password [current] [new]
│   └── token
│       ├── create                          (prompt or --name/--scopes/--expires)
│       ├── list
│       ├── info|validate|whoami            (--prompt)
│       └── revoke <id>                     (-y/--yes)
├── upload [path...]                        (--mode, --data-file, --password, --password-stdin,
│   │                                        --force, --abort, --name, --workers, --depth)
│   └── pick                                (--workers, --depth; prompts mode interactively)
├── chunks
│   ├── check [sha256]
│   └── put [path]
├── get [id|bundle.discloud]                (--download, --out, --password, --password-stdin,
│   │                                        --token, --meta, --name, --file, --all, --force)
│   └── pick [id|bundle]                    (same flags as get)
├── files
│   ├── list [bundle.discloud]              (--limit, --offset)
│   ├── get [id]                            (--token)
│   ├── inspect [id|bundle]                 (--file, --token)
│   ├── verify <path> [sha256|bundle]       (--file, --all)
│   ├── visibility [id] {public|private}    (-y)          (server-only)
│   ├── share [id]                          (--password, --clear-password, --expires,
│   │                                        --max-downloads, --clear-max-downloads, --mode)
│   ├── revoke [id]                         (-y)          (server-only)
│   ├── rotate-token [id]                   (-y)          (server-only)
│   └── delete [id|bundle]                  (-y, --file, --all, --local-only,
│                                            --keep-data-file, --password, --password-stdin)
├── admin
│   └── overview
├── health
├── ready
├── config
│   ├── get
│   ├── set [token]                         (--base, --origin, --token, --clear-token)
│   │   ├── self-upload                     (--guild-id, --channel-id, --chunk-mb,
│   │   │                                    --bot-token, --append-tokens, --clear-tokens, --clear)
│   │   └── customize                       (--upload-colors, --download-colors, --prompt-theme, --align)
│   ├── unset [base|origin|token]...        (--base, --origin, --token)
│   └── path
├── completion {bash|zsh|fish|powershell}
└── version
```

Run `discloud <command> --help` for the live flag list.

## Self-managed mode

Direct Discord upload/download without the DisCloud API. Metadata lives in a portable `.discloud` bundle.

```bash
discloud config set self-upload
discloud upload file.bin --mode self
discloud get ./file.bin.discloud
discloud files list ./file.bin.discloud
```

- Configure `selfUpload` in `config.yaml` (bot tokens, guild ID, channel ID, preferred chunk size in bytes).
- Before every self upload the CLI rechecks the guild premium tier and adjusts the effective chunk size when necessary.
- Tier limits (UI labeled MB = MiB): tier 0/1 → 10 MB, tier 2 → 50 MB, tier 3 → 100 MB.
- Tier downgrade auto-shrinks chunk size; tier upgrade prompts interactively (non-interactive keeps current size).
- Bundle protection: password (Argon2id + XChaCha20-Poly1305) or keyless obfuscation with integrity checking.
- Encrypted bundles automatically request a password in interactive terminals when no password option is supplied.
- Downloads validate a config identity hash of guild + channel + bot-token set. Bot token order does not affect bundle configuration validation.
- Multi-file uploads produce one `.discloud` bundle.

Example `selfUpload` section:

```yaml
selfUpload:
  botTokens:
    - token-1
    - token-2
  guildId: "987654321098765432"
  channelId: "123456789012345678"
  premiumTier: 2
  chunkSize: 52428800
```

## Interactive behavior

- Missing args often prompt (username, password, file id picker, upload mode, chunk size).
- Passwords prefer secure prompts or `--password-stdin` over positional args.
- TTY-aware tables, spinners, and upload/download progress bars (self-managed and server `get`; skipped with `--json`; `--meta` keeps a spinner). Ctrl+C / Esc aborts the transfer.
- Banner, tables, prompts, and status lines follow `customize.align` (`center` default under the banner axis, or `left`). Huh forms use `customize.promptTheme` (default Base 16).
- File/folder paths: pass args, or omit to prompt; `upload pick` / `get pick` open GUI dialogs after a 3s countdown (Enter opens immediately). `upload pick` asks files vs folder; folder mode lists nested paths (`folder/child/file`) with filter, Space toggle, `a` select-all, Enter to upload. `--depth N` limits folder walks (`0` = unlimited, default; `1` = top-level files only). `get pick` picks the `.discloud` (if no arg), then the save folder, then a TUI file list (same keys as upload); nested logical paths recreate folders under the save dir; existing dest files prompt overwrite (or `--force`); declining rename saves as `name.N.ext` (`N` increments while that path exists).
- `--json` disables interactive prompts that would mix with machine output.
- User abort (Ctrl+C / Esc / dialog cancel) exits quietly (status 0).
- No separate `--interactive` flag: prompts appear automatically when the terminal is interactive and values are missing.

## Upload checkpoints and resume

- Checkpoints live under the config dir `uploads/`.
- Resume matches **absolute path + size + mtime** (and chunk size).
- `discloud upload --abort <path>` cancels the server upload session for that checkpoint and clears local state.
- Mid-upload cancel (Ctrl+C / Esc) stops workers; the session remains resumable until cancelled or expired.

## Examples

Every command and flag below. Replace placeholders (`<id>`, paths, hosts) with real values. Global flags (`--base`, `--origin`, `--json`) work on any command unless noted.

### Global

```bash
discloud --base http://localhost:8080 --origin http://localhost:3000 auth me
discloud --json auth me
discloud -v
discloud version
discloud --json version
```

### Config

```bash
discloud config
discloud config get
discloud config path
discloud config --json
discloud config set --base http://localhost:8080 --origin http://localhost:3000
discloud config set --token                    # prompts for PAT (not passed on argv)
discloud config set token                      # same as --token
discloud config set --clear-token
discloud config unset base origin token
discloud config unset --base --origin --token
discloud config set self-upload                # interactive prompts
discloud config set self-upload \
  --guild-id 987654321098765432 \
  --channel-id 123456789012345678 \
  --chunk-mb 50 \
  --bot-token 'BOT_TOKEN_1' \
  --bot-token 'BOT_TOKEN_2'
discloud config set self-upload --bot-token 'EXTRA' --append-tokens
discloud config set self-upload --clear-tokens
discloud config set self-upload --clear
discloud config set customize                  # select upload/download/both/prompt theme/align/reset
discloud config set customize --reset          # both sides → built-in default gradient
discloud config set customize --reset=upload
discloud config set customize --reset=download
discloud config set customize --prompt-theme charm
discloud config set customize --align left
discloud config set customize \
  --upload-colors '#3f51b1,#f18271,#f7c978' \
  --download-colors '#0000ff,#00ff00'
```

Progress bar colors, prompt theme, and align live under `customize` in `config.yaml`. Interactive mode asks what to edit (`upload` / `download` / `both` / **prompt theme** / **align** / **reset to default**). Color paths open a **centered** HSL picker with a **live 100% preview**. Choosing **both** runs one picker and applies the same stops + longHue to upload and download. **Prompt theme** opens a live-styled list of huh themes (default **Base 16**); moving the cursor restyles the preview. **Align** chooses `center` (default, banner + content under the banner axis) or `left` (banner and content flush left). Use **prev/next** (`[`/`]` or click) between color steps; **`h` / click hex** to type `#RGB`/`#RRGGBB` directly; **`r` / reset** restores the built-in gradient; **`l` / long hue** toggles HSV long-arc blend; Enter applies the current step; Esc saves. Flags take comma-separated `#RGB` / `#RRGGBB`, `--prompt-theme`, `--align`, or `--reset[=upload|download|both]`. Empty / unset uses the built-in gradient, Base 16 prompts, and center align.

```yaml
customize:
  promptTheme: base16   # base16 | charm | dracula | catppuccin | default
  align: center         # left | center (default center)
  upload:
    progressbar:
      colors: ["#3f51b1", "#f18271", "#f7c978"]
      longHue: true   # HSV long-arc blend between stops (spectrum spin)
  download:
    progressbar:
      colors: ["#0000ff", "#00ff00"]
      longHue: false
```

Flags: `--upload-long-hue` / `--download-long-hue` / `--prompt-theme` / `--align`. In the color picker, toggle **long hue** on the nav row (or press `l`); active = purple label.
### Auth

```bash
discloud auth login
discloud auth login alice
discloud auth login alice --password-stdin <<<'secret'
discloud auth signin                           # alias of login
discloud auth logout
discloud auth signout                          # alias of logout
discloud auth signup
discloud auth signup bob --password-stdin <<<'secret'
discloud auth me
discloud auth --json me
discloud auth password                         # prompts for current + new
discloud auth password 'old' 'new'             # positional deprecated

# Personal access tokens (scopes: upload|read|manage|admin; admin also needs admin role)
discloud auth token create                     # interactive: name, scopes, optional expires
discloud auth token create --name ci --scopes upload,read,manage
discloud auth token create --name temp --scopes read --expires 2027-01-01T00:00:00Z
discloud auth token list
discloud auth token info
discloud auth token info --prompt              # type token; ignores session cookies
discloud auth token validate                   # alias of info
discloud auth token whoami                     # alias of info
discloud auth token revoke <token-id> -y
```

### Upload (server + self)

```bash
discloud upload ./file.bin
discloud upload ./file.bin --name remote.bin
discloud upload ./a.bin ./b.bin
discloud upload ./my-folder
discloud upload ./my-folder --depth 1          # top-level files only (0=unlimited)
discloud upload ./my-folder --depth 2
discloud upload --mode server ./file.bin
discloud upload --mode self ./file.bin
discloud upload --mode self ./file.bin --data-file ./out.discloud --force
discloud upload --mode self ./file.bin --password
discloud upload --mode self ./file.bin --password-stdin <<<'bundle-pass'
discloud upload --mode self ./a.bin ./b.bin --workers 8
discloud upload --abort ./file.bin             # server checkpoint only
discloud upload pick                           # GUI; files or folder → TUI; prompts mode
discloud upload pick --workers 4
discloud upload pick --depth 1
```

### Get / download

```bash
discloud get <file-id>
discloud get <file-id> --download --out ./downloaded.bin
discloud get <file-id> --password 'share-pass' --download --out ./out.bin
discloud get <file-id> --token <access-token> --out ./out.bin
discloud get <file-id> --meta                  # API JSON metadata (?json=1)
discloud get <file-id> --name display-name --out ./out.bin
discloud get ./bundle.discloud
discloud get ./bundle.discloud --out ./downloads
discloud get ./bundle.discloud --all --force
discloud get ./bundle.discloud --file photo.jpg --file notes.txt
discloud get ./bundle.discloud --password 'bundle-pass' --out ./downloads
discloud get ./bundle.discloud --password-stdin --out ./downloads <<<'bundle-pass'
discloud get pick                              # pick .discloud + folder + TUI
discloud get pick ./bundle.discloud --out ./downloads --force
discloud get pick --all --password 'bundle-pass'
```

### Files

```bash
discloud files list
discloud files list --limit 50 --offset 0
discloud files list ./bundle.discloud
discloud files get <file-id>
discloud files get <file-id> --token <access-token>
discloud files inspect <file-id>
discloud files inspect <file-id> --token <access-token>
discloud files inspect ./bundle.discloud
discloud files inspect ./bundle.discloud --file photo.jpg
discloud files verify ./downloaded.bin <sha256>
discloud files verify ./bundle.discloud
discloud files verify ./local.bin ./bundle.discloud
discloud files verify ./bundle.discloud --all
discloud files verify ./bundle.discloud --file photo.jpg
discloud files visibility <file-id> public -y
discloud files visibility <file-id> private -y
discloud files share <file-id> --password 'secret' --mode view --max-downloads 10
discloud files share <file-id> --mode download --expires 2026-12-31T23:59:59Z
discloud files share <file-id> --clear-password --clear-max-downloads
discloud files revoke <file-id> -y
discloud files rotate-token <file-id> -y
discloud files delete <file-id> -y
discloud files delete ./bundle.discloud -y --all
discloud files delete ./bundle.discloud -y --file photo.jpg
discloud files delete ./bundle.discloud -y --all --local-only
discloud files delete ./bundle.discloud -y --all --keep-data-file
discloud files delete ./bundle.discloud -y --all --password 'bundle-pass'
discloud files delete ./bundle.discloud -y --all --password-stdin <<<'bundle-pass'
```

### Chunks

```bash
discloud chunks check <sha256>
discloud chunks put ./chunk.bin
```

### Admin / health

```bash
discloud admin overview                        # admin cookie or PAT with admin scope
discloud admin --json overview
discloud health
discloud ready
discloud --json health
discloud --json ready
```

### Completion

| Shell | This session | Permanent |
| --- | --- | --- |
| **bash** | `source <(discloud completion bash)` | Linux: `discloud completion bash > /etc/bash_completion.d/discloud`<br>macOS: `discloud completion bash > $(brew --prefix)/etc/bash_completion.d/discloud` |
| **zsh** | `source <(discloud completion zsh)` | Linux: `discloud completion zsh > "${fpath[1]}/_discloud"`<br>macOS: `discloud completion zsh > $(brew --prefix)/share/zsh/site-functions/_discloud` |
| **fish** | `discloud completion fish \| source` | `discloud completion fish > ~/.config/fish/completions/discloud.fish` |
| **powershell** | `discloud completion powershell \| Out-String \| Invoke-Expression` | Add that line to `$PROFILE`, then restart the terminal |

```bash
discloud completion bash
discloud completion zsh
discloud completion fish
discloud completion powershell
```
