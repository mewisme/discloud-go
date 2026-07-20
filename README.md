# DisCloud

Unlimited cloud storage using Discord attachments.

Files are split into 8 MB chunks and stored as Discord message attachments.
Metadata lives in PostgreSQL. Signed CDN URLs are refreshed on download and
cached in Valkey so share links stay valid.

## Stack

| Layer | Tech |
| --- | --- |
| API | Go, stdlib `net/http`, pgx, valkey-go |
| Web | Next.js (App Router), TypeScript, Tailwind CSS, shadcn/ui |
| Data | PostgreSQL 17, Valkey 8 |
| Deploy | Docker Compose, GoReleaser, GitHub Actions |

## Quick start

1. Create a Discord bot, invite it to a server with permission to send messages
   and attach files, then copy a channel ID (Developer Mode → right-click
   channel → Copy Channel ID).
2. Configure env:

   ```bash
   cp .env.example .env
   ```

   Set `DISCORD_BOT_TOKEN` and `DISCORD_CHANNEL_ID`. For production behind
   Cloudflare, also set `PUBLIC_BASE_URL` to your public site origin.

3. Run:

   ```bash
   docker compose up --build -d
   ```

4. Open http://localhost:3000

   - **Upload** — drag-and-drop / browse
   - **Files** — uploads saved in this browser (`localStorage`)
   - **API** — HTTP reference for the same origin

The Go API is not published on the host. Next.js rewrites `/api/*`, `/f/*`, and
`/readyz` to it, so the browser and scripts use one origin (ready for Cloudflare).

## Development

```bash
# API (requires DATABASE_URL, VALKEY_URL, DISCORD_BOT_TOKEN, DISCORD_CHANNEL_ID)
go run ./cmd/discloud

# Web (proxies /api and /f to http://localhost:8080)
cd web && pnpm install && pnpm run dev

# Checks
go vet ./... && go test ./...
cd web && pnpm run lint && pnpm exec tsc --noEmit
```

Useful make targets: `make up`, `make down`, `make test`, `make snapshot`.

## API

All paths are relative to the site origin (e.g. `http://localhost:3000` or your
domain). Full examples live on the in-app **API** page (`/docs`).

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/upload?fileName=` | Stream whole file (raw body) |
| `GET` / `HEAD` | `/api/chunks/{sha256}` | Chunk already stored? |
| `POST` | `/api/chunks` | Upload one chunk (max 8 MB) |
| `POST` | `/api/upload/complete` | Assemble file from chunk hashes |
| `GET` | `/f/{id}[/{name}]` | Download (`Range`, `?download=1`) |
| `GET` | `/api/files` | List recent files (server) |
| `GET` | `/api/files/{id}` | File metadata |
| `GET` | `/healthz`, `/readyz` | Liveness / readiness |

The web UI uses the chunked flow (8 MB pieces, SHA-256 skip if present) so
uploads stay under proxy body limits and retries resume without re-sending
chunks already on the server.

```bash
BASE=http://localhost:3000

# Small file
curl -X POST --data-binary @file.bin "$BASE/api/upload?fileName=file.bin"

# Download
curl -OJ "$BASE/f/<fileId>?download=1"
```

## Configuration

| Variable | Required | Notes |
| --- | --- | --- |
| `DISCORD_BOT_TOKEN` | yes | Bot token(s); comma-separated list divides uploads across bots |
| `DISCORD_CHANNEL_ID` | yes | Channel that holds chunks |
| `PUBLIC_BASE_URL` | no | Share-link origin (Compose default: `http://localhost:3000`) |
| `POSTGRES_PASSWORD` | no | Compose Postgres password (default: `discloud`) |
| `DATABASE_URL` | yes\* | Postgres URL (\*set by Compose) |
| `VALKEY_URL` | yes\* | Valkey URL (\*set by Compose) |
| `PORT` | no | API listen port (default `8080`) |
| `API_URL` | no | Web → API origin for rewrites (Compose: `http://api:8080`) |

See [`.env.example`](.env.example).

## Releases

Push a `v*` tag to run GoReleaser: cross-platform binaries, checksums, changelog,
and multi-arch images on GHCR for the API and web.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). By participating you agree to the
[Code of Conduct](CODE_OF_CONDUCT.md). Security reports: [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © mewisme
