# DisCloud Web

Next.js UI for [DisCloud](../README.md): upload, manage, and share files against the Go API.

Stack: **Next.js 16** · **React 19** · **Tailwind 4** · **pnpm** · shadcn/ui (base-nova)

## Features

- **Upload** (`/`): multi-file and folder pickers; 8 MiB chunked sessions (falls back to legacy complete); IndexedDB resume (re-select the same file); queue with per-file retry/cancel
- **Files** (`/files`): owned file table; visibility; token reveal; delete
- **Inspect** (`/i/{id}`): views/downloads/bytes + share/QR
- **Account** (`/me`, `/signin`, `/signup`): session cookie auth; preferences; password change
- **API docs** (`/docs`): in-app HTTP reference
- **Install proxy**: `GET /install.sh` and `GET /install.ps1` fetch scripts from the API
- Theme toggle (system / light / dark); API health banner

Uploads need a **secure context** (`https` or `localhost`) for `crypto.subtle`.

## Quick start

From the **repo root** (API must be reachable):

```bash
# terminal 1 — full stack, or at least the API on :8080
docker compose up -d
# or: make run  (needs Discord + DATABASE_URL + VALKEY_URL + WEB_ORIGIN)

# terminal 2 — this app
cd web
pnpm install
echo 'API_URL=http://localhost:8080' > .env.local
pnpm dev
```

Open http://localhost:3000. Server `WEB_ORIGIN` must be exactly `http://localhost:3000` (CORS + cookies).

## Configuration

| Variable | Where | Default | Role |
| --- | --- | --- | --- |
| `API_URL` | Server (layout) | `http://localhost:8080` | Browser-facing API origin; injected as `self.__DISCLOUD_API__` |
| `API_UPSTREAM` | Server (install routes) | `API_URL`, then localhost | Docker-internal API for `/install.sh` / `/install.ps1` |
| `PORT` / `HOSTNAME` | Docker image | `3000` / `0.0.0.0` | Standalone Node server |

Compose sets `API_URL` (public) and `API_UPSTREAM=http://api:8080`. Locally, only `API_URL` in `web/.env.local` is usually enough.

The browser talks to `API_URL` with `credentials: "include"`. Cross-origin setups need the API `WEB_ORIGIN` to match this UI’s origin.

## Routes

| Path | Purpose |
| --- | --- |
| `/` | Uploader |
| `/files` | File list (signed-in) |
| `/i/[id]` | File inspect / share |
| `/me` | Account |
| `/signin`, `/signup` | Auth forms |
| `/docs` | API reference UI |
| `/install.sh`, `/install.ps1` | Proxied CLI installers |

## Layout

```text
web/
  src/app/           App Router pages + install routes
  src/components/    UI (uploader, files, auth, inspect, shadcn)
  src/lib/           api client, chunked upload, IndexedDB sessions, install proxy
  Dockerfile         multi-stage → Next standalone on node:22
  design.md          Geist light tokens (dark: design.dark.md)
```

## Scripts

```bash
pnpm install
pnpm dev          # next dev
pnpm build        # next build (standalone output for Docker)
pnpm start        # next start
pnpm lint         # eslint
pnpm exec tsc --noEmit
```

From repo root: `make web-dev`, `make web-build`, `make lint` (includes web checks).

## Docker

Image: `ghcr.io/mewisme/discloud-web` (built from this directory).

```bash
# from repo root
docker compose up -d web
# or build:
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d web
```

Production: set `API_URL` to the **public** API origin the browser can call. Keep `API_UPSTREAM` on the compose network hostname when install scripts are served through the UI.

## Development notes

- Package manager is **pnpm** (`packageManager` pinned in `package.json`). Use Corepack or matching pnpm.
- React Compiler is enabled (`babel-plugin-react-compiler`).
- No dedicated front-end test suite; CI runs lint, `tsc --noEmit`, and `pnpm build`.
- Product behavior (retention, privacy, Discord storage) is owned by the API — see the [root README](../README.md).

## License

Same as the monorepo: [MIT](../LICENSE).
