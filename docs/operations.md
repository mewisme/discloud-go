# Operations

## Docker Compose

Default pull-and-run stack: [`docker-compose.yml`](../docker-compose.yml).

| Service | Image | Ports | Notes |
| --- | --- | --- | --- |
| `api` | `ghcr.io/mewisme/discloud:${DISCLOUD_TAG:-latest}` | `6373` | Depends on healthy Postgres + Valkey |
| `web` | `ghcr.io/mewisme/discloud-web:${DISCLOUD_TAG:-latest}` | `3000` | `mem_limit: 256m` |
| `postgres` | `postgres:18-alpine` | internal | Volume `discloud-pgdata` |
| `valkey` | `valkey/valkey:8-alpine` | internal | No AOF/RDB (`--save "" --appendonly no`) |

Volumes:

| Volume | Mount | Contents |
| --- | --- | --- |
| `discloud-pgdata` | Postgres data | All metadata |
| `discloud-data` | API `/data` | `.app.secret`, `.visitor.secret` |

Network: bridge `discloud`. Health checks: `pg_isready` and `valkey-cli ping`.

Build from source instead of pulling GHCR:

```bash
docker compose -f docker-compose.yml -f docker-compose.build.yml up --build -d
```

Local images: `discloud-api:local`, `discloud-web:local` ([`docker-compose.build.yml`](../docker-compose.build.yml)).

## DISCLOUD_APP_SECRET and persistent data

**Persist the API data volume.** If `DISCLOUD_APP_SECRET` is unset, the API auto-generates a secret and writes it to `.app.secret` in the process working directory (Docker: `/data/.app.secret` on volume `discloud-data`). A visitor hash salt is stored as `.visitor.secret` in the same directory.

| Event | Effect |
| --- | --- |
| Lose `/data` without a fixed `DISCLOUD_APP_SECRET` env | New secret generated; existing sessions, file access tokens, and upload resume tokens become invalid |
| Change `DISCLOUD_APP_SECRET` deliberately | Same invalidation |
| Run multiple API replicas | All must share the **same** durable `DISCLOUD_APP_SECRET` (env or shared volume) |

Env `DISCLOUD_APP_SECRET` wins over the file when both are present. Minimum length: 32 characters.

## What to back up

1. **PostgreSQL** (`discloud-pgdata`) — users, files, chunks index, upload sessions, tokens, runtime `config` rows
2. **API data volume** (`discloud-data`) — `.app.secret`, `.visitor.secret` (unless you inject `DISCLOUD_APP_SECRET` via env and manage visitor salt separately)
3. Your `.env` / secrets store — Discord tokens, `DISCLOUD_CORS`, `DISCLOUD_API_URL`, optional CAPTCHA keys, optional `DISCLOUD_GUILD_ID`

Valkey is intentionally non-durable; do not rely on it for backup.

## Valkey restart

With persistence disabled:

- CDN URL cache rebuilds on next download
- Rate-limit counters reset
- Anonymous daily quota counters reset
- Auth throttle counters reset

`/readyz` fails if Valkey is down.

## Reverse proxy (production)

Path-proxy these to the API (keep the web origin for the UI):

- `/api/*`
- `/f/*`
- `/install.sh`, `/install.ps1`
- `/readyz` (and optionally `/healthz`)

Set:

- `DISCLOUD_API_URL` to the **public** API origin browsers and share links use (e.g. `https://api.example.com`)
- `DISCLOUD_CORS` to the exact UI origin (e.g. `https://files.example.com`)
- `DISCLOUD_TRUST_PROXY=true` **only** behind an edge that strips client-supplied `X-Forwarded-*` / `X-Real-IP`

Example (conceptual nginx snippet):

```nginx
# UI → Next.js (web:3000)
# API paths → Go API (api:6373)

location /api/ {
  proxy_pass http://api:6373;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
}

location /f/ {
  proxy_pass http://api:6373;
  proxy_set_header Host $host;
  proxy_set_header X-Forwarded-Proto $scheme;
  proxy_set_header X-Real-IP $remote_addr;
}

location ~ ^/install\.(sh|ps1)$ {
  proxy_pass http://api:6373;
}

location /readyz {
  proxy_pass http://api:6373;
}
```

Recreate api + web after changing `DISCLOUD_API_URL`:

```bash
docker compose up -d --force-recreate api web
```

## Cleanup jobs

The API runs periodic cleanup (~hourly):

- Deletes expired **file** rows (metadata only; Discord untouched)
- Marks abandoned open upload sessions as `expired`

See [architecture.md](architecture.md).

## CLI self-managed uploads

Operators who use `discloud upload --mode self` store Discord bot tokens in the user `config.yaml` (`selfUpload`), separate from the API server's `DISCLOUD_BOT_TOKEN`. Keep those tokens out of shell history (`config set self-upload` prompts). Bundle downloads require the same guild/channel/token set (order-independent identity hash). See [cli.md](cli.md) and [configuration.md](configuration.md).
