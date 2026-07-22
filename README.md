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

   Set `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID`. Behind Cloudflare, also set
   `API_URL` to your public **API** origin (not the `:3000` UI).

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

## Development

```bash
go run ./cmd/discloud          # needs DATABASE_URL, VALKEY_URL, Discord env
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
| `POST /api/upload?fileName=` | Whole-file upload (raw body) |
| `POST /api/chunks` · `GET/HEAD /api/chunks/{sha256}` | Chunked / resumable |
| `POST /api/upload/complete` | Assemble from chunk hashes |
| `GET /f/{id}` | Download (`Range`, `?download=1`, `?json=1` metadata). Single-chunk inline views redirect to Discord CDN |
| `GET /api/files` · `/api/files/{id}` | List / metadata |
| `GET /api/files/{id}/inspect` | Analytics + share URLs |
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
| `API_URL` | Public API origin for share links and the web UI (default `http://localhost:8080`). Recreate api+web after change — no image rebuild |
| `VISITOR_HASH_SALT` | Salt for unique-visitor hashes on inspect analytics |
| `DISCLOUD_TAG` | Image tag (default `latest`) |
| `POSTGRES_PASSWORD` | Compose DB password (default `discloud`) |

Compose sets `DATABASE_URL` and `VALKEY_URL`. See [`.env.example`](.env.example).

## Releases

Tag `v*` → GoReleaser binaries + multi-arch GHCR images for API and web.

## License

[MIT](LICENSE) © mewisme · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
