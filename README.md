# DisCloud

[![CI](https://github.com/mewisme/discloud-go/actions/workflows/ci.yml/badge.svg)](https://github.com/mewisme/discloud-go/actions/workflows/ci.yml) [![Release](https://github.com/mewisme/discloud-go/actions/workflows/release.yml/badge.svg)](https://github.com/mewisme/discloud-go/actions/workflows/release.yml) [![Go](https://img.shields.io/github/go-mod/go-version/mewisme/discloud-go?logo=go)](go.mod) [![Latest release](https://img.shields.io/github/v/release/mewisme/discloud-go?logo=github)](https://github.com/mewisme/discloud-go/releases/latest) [![GHCR](https://img.shields.io/badge/ghcr.io-discloud%20%2F%20discloud--web-blue?logo=docker)](https://github.com/mewisme/discloud-go/pkgs/container/discloud) [![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)

Self-hosted file host that stores files as Discord attachments (fixed **8 MiB** chunks), with a **Postgres** index, **Valkey** for CDN URL cache and rate/quota counters, a **Go API**, a **Next.js** UI, and a **`discloud` CLI**.

## Features

- Content-addressed chunks and resumable upload sessions (web IndexedDB + CLI checkpoints)
- Optional **self-managed** CLI mode: upload/download directly to Discord via portable `.discloud` bundles
- Public/private files, password shares, download limits, access-token rotation
- Cookie sessions and personal API tokens (`dc_…`); first user on a fresh DB is `admin`
- Compose stack: API + web + Postgres 18 + Valkey 8
- CLI via `/install.sh`, `/install.ps1`, Scoop, or Homebrew

## Architecture

```mermaid
flowchart LR
  clients[Web_CLI] --> api[Go_API]
  api --> pg[(Postgres)]
  api --> vk[(Valkey)]
  api --> discord[Discord_API]
  clients -->|download_redirect_or_proxy| cdn[Discord_CDN]
  api --> cdn
```

DisCloud uses fixed 8 MiB chunks for broad compatibility with Discord attachment uploads (max 8192 chunks ≈ 64 GiB per file). Details: [docs/architecture.md](docs/architecture.md).

## Quick start

1. Create a Discord bot with **Attach Files** permission and note a channel ID.
2. Configure env:

```bash
cp .env.example .env
# set DISCLOUD_BOT_TOKEN, DISCLOUD_CHANNEL_ID, DISCLOUD_CORS
```

3. Start the stack:

```bash
docker compose up -d
```

| | |
| --- | --- |
| UI | http://localhost:3000 |
| API | http://localhost:8080 |
| Docs | http://localhost:3000/docs |

Images: `ghcr.io/mewisme/discloud` and `ghcr.io/mewisme/discloud-web` (`DISCLOUD_TAG`, default `latest`).

Build from source:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

## CLI installation

From a running instance:

```bash
curl -fsSL https://files.example.com/install.sh | sh   # macOS / Linux
irm https://files.example.com/install.ps1 | iex        # Windows
```

Or:

```bash
scoop install mew/discloud-cli
brew install --cask discloud-cli
go build -o discloud-cli ./cmd/discloud-cli
```

The released command is `discloud`. The API container entrypoint is `/discloud`. Full command tree: [docs/cli.md](docs/cli.md).

## Basic usage

Web: open http://localhost:3000 — upload files or folders, manage visibility, copy share links. Account at `/me`; inspect at `/i/<file-id>`; admin at `/admin`.

```bash
discloud config set --base https://api.example.com --origin https://files.example.com
discloud auth login
discloud upload ./file.bin
discloud files list
discloud files share <file-id> --password 'secret-pass' --mode view --max-downloads 10
discloud get <file-id> --password 'secret-pass' --download --out ./downloaded.bin
discloud files verify ./downloaded.bin <sha256>
```

Whole-file digests use **discloud-sha256-v1** (equals `sha256sum` only for single-chunk files ≤ 8 MiB). See [docs/file-integrity.md](docs/file-integrity.md).

## Operational notes

**Deleting a file from DisCloud does not delete Discord-hosted attachments.** Cancel marks an upload session `cancelled` (row kept); delete/expiry remove Postgres metadata only. Discord lifetime is outside this app.

**Persist `/data`.** If `DISCLOUD_APP_SECRET` is unset, the API writes `.app.secret` (and `.visitor.secret`) under the process cwd — Docker volume `discloud-data` → `/data`. Losing or changing the secret invalidates sessions, file access tokens, and upload resume tokens. Multiple API replicas must share the same durable secret.

**Back up** Postgres (`discloud-pgdata`) and the API data volume. Valkey persistence is disabled on purpose; rate-limit/quota counters reset on restart.

Fuller ops: [docs/operations.md](docs/operations.md). Config: [docs/configuration.md](docs/configuration.md).

## Documentation

| Doc | Contents |
| --- | --- |
| [Architecture](docs/architecture.md) | Domain model, chunking, upload lifecycle, retention |
| [Configuration](docs/configuration.md) | Environment variables and fixed limits |
| [API](docs/api.md) | Routes, auth, uploads, downloads |
| [CLI](docs/cli.md) | Command tree, install, checkpoints |
| [Security](docs/security.md) | Cookies, tokens, CSRF, secrets |
| [Operations](docs/operations.md) | Compose, backup, reverse proxy, `DISCLOUD_APP_SECRET` |
| [File integrity](docs/file-integrity.md) | `discloud-sha256-v1` construction and verify |

Also: [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), [web/README.md](web/README.md).

## Development

Requires **Go 1.26.3+** (see `go.mod`), Postgres, Valkey, and Discord credentials for a local API.

```bash
cp .env.example .env
make up          # compose stack
make run         # go run ./cmd/discloud
make web-dev     # cd web && pnpm run dev
make test        # go vet + go test
make lint        # gofmt check + web lint/tsc
make build       # dist/discloud + dist/discloud-cli
```

CI (push/PR): gofmt, `go vet`, `go test`, web lint/typecheck/build, Docker image builds.  
Release: tag `v*` → GoReleaser (CLI + GHCR `discloud` / `discloud-web`).

## License

[MIT](LICENSE) · [Code of Conduct](CODE_OF_CONDUCT.md) · [Security](SECURITY.md)
