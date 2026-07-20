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
   `PUBLIC_BASE_URL` to your public origin.

3. Run:

   ```bash
   docker compose up -d
   ```

   Open [http://localhost:3000](http://localhost:3000).

Images come from `ghcr.io/mewisme/discloud` and `discloud-web` (`DISCLOUD_TAG`,
default `latest`). Build from source instead:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

The API is not published on the host. Next.js rewrites `/api/*`, `/f/*`, and
`/readyz` to it — one origin for the browser and Cloudflare.

## Development

```bash
go run ./cmd/discloud          # needs DATABASE_URL, VALKEY_URL, Discord env
cd web && pnpm i && pnpm dev  # proxies API to :8080
```

```bash
make test                     # go vet + go test
make up / make up-build / make down
```

## API

Use the site origin (`http://localhost:3000` or your domain). Full reference:
[/docs](http://localhost:3000/docs).

| | |
| --- | --- |
| `POST /api/upload?fileName=` | Whole-file upload (raw body) |
| `POST /api/chunks` · `GET/HEAD /api/chunks/{sha256}` | Chunked / resumable |
| `POST /api/upload/complete` | Assemble from chunk hashes |
| `GET /f/{id}` | Download (`Range`, `?download=1`) |
| `GET /api/files` · `/api/files/{id}` | List / metadata |
| `GET /api/info` | Bots / upload worker hint |
| `GET /healthz` · `/readyz` | Health |

```bash
curl -X POST --data-binary @file.bin "$BASE/api/upload?fileName=file.bin"
curl -OJ "$BASE/f/<fileId>?download=1"
```

## Config

| Variable | Notes |
| --- | --- |
| `DISCORD_BOT_TOKEN` | Required. Comma-separated tokens → parallel Discord uploads (one worker per bot) |
| `DISCORD_CHANNEL_ID` | Required. Channel that holds chunks |
| `PUBLIC_BASE_URL` | Share-link origin (Compose default `http://localhost:3000`) |
| `DISCLOUD_TAG` | Image tag (default `latest`) |
| `POSTGRES_PASSWORD` | Compose DB password (default `discloud`) |

Compose sets `DATABASE_URL` and `VALKEY_URL`. See [`.env.example`](.env.example).

## Releases

Tag `v*` → GoReleaser binaries + multi-arch GHCR images for API and web.

## License

[MIT](LICENSE) © mewisme · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
