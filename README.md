# DisCloud Go

Unlimited cloud storage backed by Discord attachments — a Go rewrite of
[mewisme/discloud](https://github.com/mewisme/discloud) with a Next.js frontend.

Files are streamed in, split into 8 MB chunks, and uploaded concurrently to a
Discord channel as message attachments. Metadata lives in PostgreSQL; signed
CDN URLs (which Discord expires after ~24h) are refreshed on demand and cached
in Valkey, so share links never go stale.

## Stack

- **API**: Go 1.26, stdlib `net/http`, `pgx/v5`, `valkey-go`
- **Frontend**: Next.js 16 (App Router, RSC), TypeScript, Tailwind CSS v4, shadcn/ui
- **Infra**: Docker Compose (API, web, PostgreSQL 17, Valkey 8), GoReleaser, GitHub Actions

## Quick start

1. Create a Discord bot, invite it to a server, and copy a channel ID
   (see the [original setup guide](https://github.com/mewisme/discloud#setup-guide)).
2. Copy the env file and fill in the two Discord values:

   ```bash
   cp .env.example .env
   ```

3. Start everything:

   ```bash
   docker compose up --build -d
   ```

   Frontend: http://localhost:3000 — API: http://localhost:8080

## Development

```bash
# API (needs DATABASE_URL, VALKEY_URL, DISCORD_* env vars)
go run ./cmd/discloud

# Frontend (proxies /api and /f to localhost:8080)
cd web && npm install && npm run dev

# Tests and checks
go vet ./... && go test ./...
cd web && npm run lint && npx tsc --noEmit
```

## API

| Method | Path                  | Description                                             |
| ------ | --------------------- | ------------------------------------------------------- |
| POST   | `/api/upload?fileName=` | Streaming upload; body is the raw file bytes           |
| GET    | `/f/{id}[/{name}]`    | Download; supports `Range` and `?download=1`            |
| GET    | `/api/files`          | Recent uploads                                          |
| GET    | `/api/files/{id}`     | File metadata                                           |
| GET    | `/healthz` / `/readyz`| Liveness / readiness (pings PostgreSQL and Valkey)      |

## Configuration

| Variable             | Required | Description                                  |
| -------------------- | -------- | -------------------------------------------- |
| `DISCORD_BOT_TOKEN`  | yes      | Bot token used to post chunks                 |
| `DISCORD_CHANNEL_ID` | yes      | Channel that stores the chunks                |
| `DATABASE_URL`       | yes      | PostgreSQL connection string                  |
| `VALKEY_URL`         | yes      | Valkey address (`valkey://host:port`)         |
| `PORT`               | no       | API listen port (default `8080`)              |
| `PUBLIC_BASE_URL`    | no       | Origin used in generated share links          |
| `API_URL`            | no       | (frontend) API origin for rewrites/SSR fetches |

## Releases

Tagging `v*` triggers GoReleaser: cross-platform binaries (linux/darwin/windows,
amd64/arm64), checksums, a grouped changelog, and multi-arch API images pushed
to GHCR, plus a multi-arch frontend image.
