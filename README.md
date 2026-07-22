# DisCloud

Cloud storage backed by Discord attachments. Files are chunked (8 MB), stored
as message attachments, indexed in PostgreSQL, and served through refreshed CDN
URLs (cached in Valkey).

**Stack:** Go API · Next.js · Postgres 18 · Valkey 8 · Docker Compose

## Quick start

1. Create a Discord bot that can send messages and attach files. Invite it to a
   server and copy a channel ID (Developer Mode → Copy Channel ID).

2. Configure:

   ```bash
   cp .env.example .env
   ```

   Set `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID`. Keep
   `WEB_ORIGIN=http://localhost:3000` for local UI. `APP_SECRET` is optional
   (≥32 chars if set); when omitted the API writes `.app.secret` automatically.

3. Run:

   ```bash
   docker compose up -d
   ```

   UI: [http://localhost:3000](http://localhost:3000) · API: [http://localhost:8080](http://localhost:8080)

Images come from `ghcr.io/mewisme/discloud` and `discloud-web` (`DISCLOUD_TAG`,
default `latest`). Build from source:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Point Cloudflare (or any reverse proxy) at the API on **:8080** for `/api/*`,
`/f/*`, `/install.sh`, `/install.ps1`, and `/readyz`. Set `API_URL` to that
public API origin and recreate the web container after changing it.

## Auth, visibility, retention

- **Public by default.** Only an owned file can become private. Anonymous
  uploads stay public.
- **Accounts.** Username + password (`POST /api/auth/signup|signin`). Username
  is set at signup only (3–32 chars, `[a-z0-9][a-z0-9_-]*`). First account on a
  fresh DB is `admin`. Session cookie: `discloud_session` (HttpOnly,
  `SameSite=Lax`, 30d; `Secure` when `WEB_ORIGIN` is HTTPS).
- **Private tokens.** Making a file private returns a one-time `accessToken`.
  Pass `?token=` or `X-File-Token`. Rotate to recover; public clears the token.
  Tokens never grant manage (visibility / rotate / delete).
- **Retention.** Anonymous: **7 days**. Signed-in: **30 days**. A full
  `?download=1` (not HEAD, not Range) extends by 7 days, capped at 30 days from
  now. Cleanup deletes expired Postgres rows only.
- **Delete = DB only.** Discord attachments are never deleted.
- **Same-site cookies.** API and UI must share a site in production (path-proxy
  the API under the UI origin). Localhost different ports are fine.

## Development

```bash
go run ./cmd/discloud          # needs DATABASE_URL, VALKEY_URL, Discord env, WEB_ORIGIN
cd web && pnpm i && pnpm dev  # API_URL in web/.env.local (default :8080)
```

```bash
make test                     # go vet + go test
make up / make up-build / make down
```

## CLI

Installers are served by the API (UI proxies `/install.sh` and `/install.ps1`)
with `DISCLOUD_BASE` / `DISCLOUD_ORIGIN` baked from `API_URL` / `WEB_ORIGIN`:

```bash
# macOS / Linux
curl -fsSL http://localhost:3000/install.sh | sh
```

```powershell
# Windows
irm http://localhost:3000/install.ps1 | iex
```

```powershell
scoop bucket add mew https://github.com/mewisme/scoop-mew
scoop install mew/discloud-cli
```

```bash
brew tap mewisme/mew
brew install --cask discloud-cli
```

Pin with `DISCLOUD_VERSION=vX.Y.Z`. Uninstall (Unix):
`curl -fsSL http://localhost:3000/install.sh | sh -s -- --uninstall`.

```bash
discloud auth signin you secret123
discloud upload ./file.bin
discloud files list
discloud config
```

PATH `discloud` from the installer is the **client**. The API binary in Docker
is `/discloud` and is separate. Config: env → `~/.config/discloud/config.json`
→ localhost defaults. Cookies: `~/.config/discloud/cookies`.

## API

Docs UI: [/docs](http://localhost:3000/docs). Base: `http://localhost:8080`.

| Endpoint | Notes |
| --- | --- |
| `POST /api/auth/signup\|signin\|signout` · `GET /api/auth/me` | Session cookie |
| `PATCH /api/auth/preferences` | `{ "defaultVisibility": "public"\|"private" }` |
| `POST /api/auth/password` | Revokes all sessions, re-issues cookie |
| `POST /api/upload?fileName=` | Whole-file upload (raw body) |
| `POST /api/chunks` · `GET/HEAD /api/chunks/{sha256}` | Chunked / resumable |
| `POST /api/upload/complete` | Assemble from chunk hashes |
| `GET /f/{id}` | Download (`Range`, `?download=1`, `?json=1`, `?token=`) |
| `GET /api/files` | Auth — owner list |
| `GET /api/files/{id}` · `/inspect` | Metadata / analytics |
| `PATCH /api/files/{id}/visibility` | Owner/admin; private returns `accessToken` |
| `POST /api/files/{id}/access-token/rotate` | Owner/admin; private only |
| `DELETE /api/files/{id}` | Owner/admin; 204; Postgres only |
| `GET /api/info` | Bots / upload worker hint |
| `GET /install.sh` · `/install.ps1` | CLI installers |
| `GET /healthz` · `/readyz` | Health |

```bash
export BASE=http://localhost:8080
curl -X POST --data-binary @file.bin "$BASE/api/upload?fileName=file.bin"
curl -OJ "$BASE/f/<fileId>?download=1"
```

## Config

| Variable | Notes |
| --- | --- |
| `DISCORD_BOT_TOKEN` | Required. Comma-separated → parallel uploads |
| `DISCORD_CHANNEL_ID` | Required. Channel that holds chunks |
| `APP_SECRET` | Optional, ≥32 chars if set. HMAC root; else `.app.secret` |
| `WEB_ORIGIN` | Required absolute origin (no path). CORS + cookie `Secure` |
| `TRUST_PROXY` | Honor `X-Forwarded-For` / `X-Real-IP` behind a trusted edge |
| `API_URL` | Public API origin for share links / web UI |
| `DISCLOUD_TAG` | Image tag (default `latest`) |
| `POSTGRES_PASSWORD` | Compose DB password (default `discloud`) |

On-disk secrets (cwd; Docker `data` volume → `/data`):

- `.app.secret` — HMAC root when `APP_SECRET` is unset
- `.visitor.secret` — unique-visitor hash salt

Compose sets `DATABASE_URL` and `VALKEY_URL`. See [`.env.example`](.env.example).

## Releases

Tag `v*` → GoReleaser: multi-OS `discloud-cli`, Scoop/Homebrew manifests, and
multi-arch GHCR images for API and web.

## License

[MIT](LICENSE) © mewisme · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
