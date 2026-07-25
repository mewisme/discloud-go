# Configuration

Environment variables for the API server (`cmd/discloud`) and related Compose/web settings. Source of truth: [`internal/config/config.go`](../internal/config/config.go) and [`.env.example`](../.env.example).

## Required (API)

| Variable | Description |
| --- | --- |
| `DISCLOUD_BOT_TOKEN` | One token, or comma-separated tokens for round-robin uploads |
| `DISCLOUD_CHANNEL_ID` | Channel where attachments are posted |
| `DISCLOUD_CORS` | Exact browser origin for CORS + cookies (no path/query), e.g. `http://localhost:3000` |
| `DISCLOUD_DATABASE_URL` | Postgres (`postgres://` / `postgresql://`) or SQLite (`sqlite:` / `file:`) |
| `DISCLOUD_VALKEY_URL` | Required for Postgres (`valkey://` / `redis://`). Optional for SQLite (empty → in-memory cache) |

Compose injects Postgres + Valkey URLs for containers. Local `discloud server install` writes a SQLite DSN and leaves Valkey empty.

`DISCLOUD_CORS` using `https` sets the session cookie **Secure** flag.

## Optional (API)

| Variable | Default | Description |
| --- | --- | --- |
| `DISCLOUD_HOST` | empty (all interfaces) | Listen host; local installs use `127.0.0.1` |
| `DISCLOUD_PORT` | `6373` | Listen port |
| `DISCLOUD_API_URL` | (derive from request) | Public API origin for share links; **not** the UI URL |
| `DISCLOUD_APP_SECRET` | auto `.app.secret` | HMAC root (≥32 chars). Env wins over file |
| `DISCLOUD_DATA_DIR` | (cwd for Docker) | Persistent data dir for DB path defaults, `.app.secret`, `.visitor.secret` |
| `DISCLOUD_CONFIG_DIR` | (exe dir / unset) | Directory containing `server.yaml` and `.env` |
| `DISCLOUD_LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `DISCLOUD_TRUST_PROXY` | off | Honor `X-Forwarded-For` / `X-Real-IP` only behind an edge that strips client values |
| `DISCLOUD_RL_UPLOAD` | `60` | Upload RL per IP (+ per user when signed in). `0` = off |
| `DISCLOUD_RL_DOWNLOAD` | `120` | Download RL per IP on `/f/{id}`. `0` = off |
| `DISCLOUD_MAX_USER_BYTES` | `0` | Owned storage cap (bytes). `0` = unlimited |
| `DISCLOUD_MAX_ANON_UPLOADS` | `50` | Anon session creates per IP/day. `0` = unlimited |
| `DISCLOUD_MAX_ANON_BYTES` | `2147483648` (2 GiB) | Anon completed bytes per IP/day. `0` = unlimited |
| `DISCLOUD_MAX_RAW_UPLOAD` | `0` | Hard cap on `POST /api/upload` body. `0` = max file size |
| `DISCLOUD_CAPTCHA_SECRET` | — | Turnstile secret; enables anon upload CAPTCHA when set |

Byte-valued env vars are plain integers (no `GiB` suffix). Empty → default.

### Backend combinations

| Database | Valkey | Result |
| --- | --- | --- |
| Postgres | set | Valid (production) |
| Postgres | empty | Invalid |
| SQLite | set | Valid |
| SQLite | empty | Valid → in-memory cache |

Canonical local SQLite DSN: `sqlite:<absolute-path>/discloud.db` (forward slashes in the DSN string).

## Local server

`discloud server install` writes canonical `server.yaml` under the install `config/` directory (beside a presented copy in `current/`). Precedence when the server starts:

```text
process environment > .env > server.yaml > built-in defaults
```

Merging does not mutate the parent process environment. CLI-managed starts pass absolute `DISCLOUD_CONFIG_DIR` and `DISCLOUD_DATA_DIR`.

Basic YAML keys: `host`, `port`, `botTokens`, `channelId`, `cors`, `apiUrl`, `logLevel`. Advanced: `databaseUrl`, `valkeyUrl` (CLI: `--database-url` / `--valkey-url`). Never print raw bot tokens without `--show-secret`.

Backup recommendation: copy `<dataDir>/discloud.db` (and `-wal`/`-shm` if present) while the server is stopped.

Docker/Compose PostgreSQL + Valkey production path is unchanged.

## Compose / web

| Variable | Default | Description |
| --- | --- | --- |
| `DISCLOUD_TAG` | `latest` | GHCR image tag for `api` and `web` |
| `POSTGRES_PASSWORD` | `discloud` | Postgres password |
| `DISCLOUD_API_URL` | `http://localhost:6373` | Browser-facing API origin (web + API share links) |
| `DISCLOUD_API_UPSTREAM` | `http://api:6373` (compose) | Server-side proxy target for `/install.sh` and `/install.ps1` |
| `DISCLOUD_CAPTCHA_KEY` | — | Turnstile site key for the web UI (`GET /api/captcha` on the Next.js app) |

`DISCLOUD_API_URL` must be reachable from the browser. `DISCLOUD_API_UPSTREAM` is Docker-internal and used only by the web container to fetch install scripts.

After changing `DISCLOUD_API_URL`, recreate api + web:

```bash
docker compose up -d --force-recreate api web
```

## CLI client

| Variable | Description |
| --- | --- |
| `DISCLOUD_BASE` | API origin |
| `DISCLOUD_ORIGIN` | Must match server `DISCLOUD_CORS` (CSRF) when using cookie login |
| `DISCLOUD_TOKEN` | Personal access token (`dc_…`). Used only when the session cookie jar is empty |

Resolution order: env → `config.yaml` → `http://localhost:6373` / `http://localhost:3000`. Flags `--base` / `--origin` override after load. Session cookie jar wins over `DISCLOUD_TOKEN`. Legacy `config.json` migrates automatically to `config.yaml`. See [cli.md](cli.md).

### Self-upload (`selfUpload` in `config.yaml`)

| Field | Description |
| --- | --- |
| `botTokens` | Discord bot tokens (array). Order does not affect config identity. |
| `guildId` | Guild ID (string) used for premium-tier checks |
| `channelId` | Channel ID (string) for chunk uploads |
| `premiumTier` | Cached guild premium tier `0`–`3` (refreshed before each self upload) |
| `chunkSize` | Preferred chunk size in **bytes** (UI prompts use MB = MiB) |

Before every self-managed upload, the CLI rechecks the guild premium tier and adjusts the effective chunk size when necessary.

### Customize (`customize` in `config.yaml`)

| Field | Description |
| --- | --- |
| `promptTheme` | Huh form theme: `base16` (default), `charm`, `dracula`, `catppuccin`, `default` |
| `align` | Banner + tables + prompts: `left` (default) or `center` |
| `upload.progressbar.colors` | Hex stops for upload progress gradient |
| `upload.progressbar.longHue` | HSV long-arc blend between upload stops |
| `download.progressbar.colors` | Hex stops for download progress gradient |
| `download.progressbar.longHue` | HSV long-arc blend between download stops |

Set via `discloud config set customize` (interactive or flags). Details: [cli.md](cli.md).

## Fixed product limits (not env)

| Limit | Value |
| --- | --- |
| Chunk size | 8 MiB |
| Max chunks / file | 8192 (~64 GiB) |
| Anon / authed file retention | 7d / 30d |
| Download extend | +7d (cap 30d from now) |
| Upload session TTL | anon 24h / auth 48h |
| Max open upload sessions | 20 |
| Max API tokens / user | 20 |
| Auth rate limit | 10 attempts / 15 minutes |
| Session cookie TTL | 30 days |
| File unlock cookie TTL | 1 hour |

## Local vs Docker

| Concern | Docker Compose | `discloud server` (local) | `go run ./cmd/discloud` |
| --- | --- | --- | --- |
| Database | Postgres (required) | SQLite by default | You set `DISCLOUD_DATABASE_URL` |
| Cache | Valkey (required) | In-memory if Valkey empty | Valkey required unless SQLite |
| Bind | all interfaces | `127.0.0.1` by default | `DISCLOUD_HOST` / port |
| Secrets | `/data` volume | `DISCLOUD_DATA_DIR` | Process cwd unless `DISCLOUD_DATA_DIR` |

See also [operations.md](operations.md) for `DISCLOUD_APP_SECRET` persistence and reverse-proxy notes.
