# DisCloud

Cloud storage backed by Discord attachments. Files are chunked (8 MB), stored
as message attachments, indexed in PostgreSQL, and served through refreshed CDN
URLs (cached in Valkey).

**Stack:** Go API · Next.js · Postgres 17 · Valkey 8 · Docker Compose

## Quick start

1. Create a Discord bot with permission to send messages and attach files.
   Invite it to a server and copy a channel ID (Developer Mode → Copy Channel ID).

2. Configure:

   ```bash
   cp .env.example .env
   ```

   Set `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID`. Keep
   `WEB_ORIGIN=http://localhost:3000` for local UI. `APP_SECRET` is optional
   (≥32 chars if set); when omitted the API writes `.app.secret` automatically.
   Behind Cloudflare, also set `API_URL` to your public **API** origin (not the
   `:3000` UI).

3. Run:

   ```bash
   docker compose up -d
   ```

   UI: [http://localhost:3000](http://localhost:3000) · API: [http://localhost:8080](http://localhost:8080).

Images come from `ghcr.io/mewisme/discloud` and `discloud-web` (`DISCLOUD_TAG`,
default `latest`). Build from source instead:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

The API is published on **:8080**. Point Cloudflare at it for `/api/*`, `/f/*`,
and `/readyz`. The web UI on **:3000** calls that origin via runtime
`API_URL` (no rebuild — recreate the web container after changing it).

## Auth, visibility, retention

- **Public by default.** Uploads are public; only an owned file can be switched
  to private. Anonymous uploads stay public and cannot be privatized.
- **Accounts.** Email + password (`POST /api/auth/signup|signin`). First account
  on a fresh DB is `admin`; later accounts are `user`. Session cookie:
  `discloud_session` (HttpOnly, `SameSite=Lax`, 30d; `Secure` when
  `WEB_ORIGIN` is HTTPS).
- **Private tokens.** Making a file private returns a one-time `accessToken`
  (shown once in the UI). Pass it as `?token=` or `X-File-Token`. Rotate to
  recover; switching back to public invalidates the token. Tokens never grant
  manage (visibility / rotate / delete).
- **Retention.** Anonymous files expire after **7 days**; signed-in uploads after
  **30 days**. A full `?download=1` (not HEAD, not Range) extends expiry by
  7 days, capped at 30 days from now. A cleanup worker deletes expired rows
  from Postgres only.
- **Delete = DB only.** Soft product delete removes Postgres metadata (cascades
  chunks/events/visitors). Discord attachments are **never** deleted.
- **Same-site cookies.** `SameSite=Lax` needs the API and UI on the same site.
  Localhost different ports are fine. In production, path-proxy the API under
  the UI origin (or otherwise keep them same-site).

## Development

```bash
go run ./cmd/discloud          # needs DATABASE_URL, VALKEY_URL, Discord env, WEB_ORIGIN
cd web && pnpm i && pnpm dev  # API_URL in web/.env.local (default :8080)
```

```bash
make test                     # go vet + go test
make up / make up-build / make down
```

## API

API is on **:8080** (`http://localhost:8080` or your public API origin). Docs UI:
[/docs](http://localhost:3000/docs).

| | |
| --- | --- |
| `POST /api/auth/signup\|signin\|signout` · `GET /api/auth/me` | Session cookie auth |
| `POST /api/upload?fileName=` | Whole-file upload (raw body); optional session → ownership + 30d retention |
| `POST /api/chunks` · `GET/HEAD /api/chunks/{sha256}` | Chunked / resumable |
| `POST /api/upload/complete` | Assemble from chunk hashes |
| `GET /f/{id}` | Download (`Range`, `?download=1`, `?json=1`, `?token=`). Single-chunk may 302 to CDN |
| `GET /api/files` | **Auth required** — owner list |
| `GET /api/files/{id}` · `/inspect` | Metadata / analytics (private needs session or token) |
| `PATCH /api/files/{id}/visibility` | Owner/admin; private returns one-time `accessToken` |
| `POST /api/files/{id}/access-token/rotate` | Owner/admin; private only |
| `DELETE /api/files/{id}` | Owner/admin; 204; Postgres only |
| `GET /api/info` | Bots / upload worker hint |
| `GET /healthz` · `/readyz` | Health |

```bash
export BASE=http://localhost:8080
curl -X POST --data-binary @file.bin "$BASE/api/upload?fileName=file.bin"
curl -OJ "$BASE/f/<fileId>?download=1"
```

## Config

| Variable | Notes |
| --- | --- |
| `DISCORD_BOT_TOKEN` | Required. Comma-separated tokens → parallel Discord uploads (one worker per bot) |
| `DISCORD_CHANNEL_ID` | Required. Channel that holds chunks |
| `APP_SECRET` | Optional, ≥32 chars if set. HMAC keys for sessions and file tokens. Env wins over `.app.secret`; if unset, auto-generated there |
| `WEB_ORIGIN` | Required absolute `http`/`https` origin (no path). CORS allowlist + cookie `Secure` |
| `API_URL` | Public API origin for share links and the web UI (default `http://localhost:8080`). Recreate api+web after change — no image rebuild |
| `DISCLOUD_TAG` | Image tag (default `latest`) |
| `POSTGRES_PASSWORD` | Compose DB password (default `discloud`) |

Secrets on disk (cwd; Docker `data` volume → `/data`):

- `.app.secret` — app HMAC root when `APP_SECRET` env is unset
- `.visitor.secret` — unique-visitor hash salt (legacy `.visitor_hash_salt` is renamed once)

Compose sets `DATABASE_URL` and `VALKEY_URL`. See [`.env.example`](.env.example).

## Migration notes

- `0006_auth_ownership.sql` adds `users`, `sessions`, and file columns
  (`owner_user_id`, `visibility`, `access_token_hash`, `expires_at`). Existing
  files are backfilled as public with `expires_at = created_at + 7 days`.
- No Discord-side migration; content-addressed `chunk_store` rows are not dropped
  solely because a logical file expired or was deleted.

## Releases

Tag `v*` → GoReleaser binaries + multi-arch GHCR images for API and web.

## License

[MIT](LICENSE) © mewisme · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
