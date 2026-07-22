# DisCloud

[![CI](https://github.com/mewisme/discloud-go/actions/workflows/ci.yml/badge.svg)](https://github.com/mewisme/discloud-go/actions/workflows/ci.yml) [![Release](https://github.com/mewisme/discloud-go/actions/workflows/release.yml/badge.svg)](https://github.com/mewisme/discloud-go/actions/workflows/release.yml) [![Go](https://img.shields.io/github/go-mod/go-version/mewisme/discloud-go?logo=go)](go.mod) [![Latest release](https://img.shields.io/github/v/release/mewisme/discloud-go?logo=github)](https://github.com/mewisme/discloud-go/releases/latest) [![GHCR](https://img.shields.io/badge/ghcr.io-discloud%20%2F%20discloud--web-blue?logo=docker)](https://github.com/mewisme/discloud-go/pkgs/container/discloud) [![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)

File hosting on Discord attachments — 8 MB chunks, Postgres index, Valkey CDN cache.

Go API · Next.js · Postgres · Valkey · Compose

## Quick start

1. Discord bot with attach permission + a channel ID  
2. `cp .env.example .env` → set `DISCORD_BOT_TOKEN`, `DISCORD_CHANNEL_ID`, `WEB_ORIGIN`  
3. `docker compose up -d`

| | |
| --- | --- |
| UI | http://localhost:3000 |
| API | http://localhost:8080 |
| Docs | http://localhost:3000/docs |

Images: `ghcr.io/mewisme/discloud` + `discloud-web` (`DISCLOUD_TAG`, default `latest`).

```bash
# build from source
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Production: path-proxy `/api/*`, `/f/*`, `/install.*`, `/readyz` to the API; set `API_URL` to that public origin.

## Behavior

- Public by default; private only for owned files (one-time `accessToken`)
- Anonymous retention **7d** · signed-in **30d** · full `?download=1` extends +7d (cap 30d)
- Delete removes Postgres rows only — Discord attachments stay
- Session cookie `discloud_session`; first user on a fresh DB is `admin`

## CLI

```bash
# install (from a running DisCloud)
curl -fsSL https://your.app/install.sh | sh          # macOS / Linux
irm https://your.app/install.ps1 | iex               # Windows
scoop install mew/discloud-cli                       # or: brew install --cask discloud-cli
```

```bash
discloud config set --base https://api.example.com --origin https://app.example.com
discloud auth login
discloud upload ./file.bin
discloud files list
```

Config order: flags → env → `config.json` → localhost.  
See `discloud config --help`. Client binary is `discloud`; Docker API is `/discloud`.

## Develop

```bash
go run ./cmd/discloud                # needs Discord + DATABASE_URL + VALKEY_URL + WEB_ORIGIN
cd web && pnpm i && pnpm dev         # API_URL in web/.env.local
make test                            # go vet + go test
```

Env reference: [`.env.example`](.env.example). Tag `v*` → CLI + GHCR via GoReleaser.

## License

[MIT](LICENSE) · [Contributing](CONTRIBUTING.md) · [Security](SECURITY.md) · [Code of Conduct](CODE_OF_CONDUCT.md)
