# API

Base URL defaults to `http://localhost:6373` (or your public `DISCLOUD_API_URL`). The in-app UI also hosts HTTP notes at `/docs`.

## Authentication

| Mechanism | How | Notes |
| --- | --- | --- |
| Session cookie | `discloud_session` | Browser calls need matching `Origin` (`DISCLOUD_CORS`). Cookie sessions have full access (no scope checks). |
| API token | `Authorization: Bearer dc_…` | Scopes: `upload`, `read`, `manage`, `admin`. Cookie jar is checked first; Bearer is fallback. |
| Private file access token | `?token=` or `X-File-Token` | One-time share token for private files (rotatable). |
| Share password | Unlock cookie or `X-File-Password` | After visibility/token gate; missing/wrong → 401 |

`admin` scope on a PAT also requires the user `role=admin`. Cookie session as `role=admin` is enough without an `admin` scope.

## Health and info

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/healthz` | Liveness (`ok`) |
| `GET` | `/readyz` | Postgres + Valkey ping |
| `GET` | `/api/info` | `chunkSize`, upload session support, optional captcha hint |

```bash
curl -s https://api.example.com/api/info
# {"chunkSize":8388608,"uploads":{"sessions":true,"maxFileSize":68719476736}}
```

## Auth routes

| Method | Path |
| --- | --- |
| `POST` | `/api/auth/signup` |
| `POST` | `/api/auth/signin` |
| `POST` | `/api/auth/signout` |
| `GET` | `/api/auth/me` |
| `PATCH` | `/api/auth/preferences` |
| `POST` | `/api/auth/password` |
| `POST` | `/api/auth/tokens` |
| `GET` | `/api/auth/tokens` |
| `DELETE` | `/api/auth/tokens/{id}` |

First signup on an empty database gets `role=admin` (Postgres advisory lock). Later signups are `user`.

## Upload sessions

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/uploads` | Create session → `uploadId` + `resumeToken` |
| `GET` | `/api/uploads/{id}` | Progress / registered parts |
| `PUT` | `/api/uploads/{id}/parts/{idx}` | Register one part `{ "hash": "…" }` |
| `POST` | `/api/uploads/{id}/parts` | Batch register parts |
| `POST` | `/api/uploads/{id}/complete` | Assemble file (idempotent; optional `fileSha256` hint) |
| `DELETE` | `/api/uploads/{id}` | Cancel — marks status `cancelled`, does **not** delete the row |

Resume auth: `X-Upload-Token` or `?token=`.

### Chunks

| Method | Path |
| --- | --- |
| `GET` / `HEAD` | `/api/chunks/{hash}` |
| `POST` | `/api/chunks` |

### Legacy upload

| Method | Path |
| --- | --- |
| `POST` | `/api/upload` |
| `POST` | `/api/upload/complete` |

## Files

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/files` | List owned files (`read` scope) |
| `GET` | `/api/files/{id}` | Metadata |
| `GET` | `/api/files/{id}/inspect` | Analytics |
| `PATCH` | `/api/files/{id}/visibility` | `public` / `private` |
| `PATCH` | `/api/files/{id}/share` | Password, expiry, max downloads, `view`\|`download` mode |
| `POST` | `/api/files/{id}/unlock` | Browser unlock cookie |
| `POST` | `/api/files/{id}/revoke` | Force-expire share |
| `POST` | `/api/files/{id}/access-token/rotate` | New private access token |
| `DELETE` | `/api/files/{id}` | Metadata-only delete |

## Admin

| Method | Path |
| --- | --- |
| `GET` | `/api/admin/overview` | Storage, users, uploads, traffic, bot count, Postgres/Valkey health |

Requires admin role (cookie) or PAT with `admin` scope plus admin role.

## Downloads and install

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/f/{id}` | Download / view (optional `/{name…}` suffix) |
| `GET` | `/f/{id}?download=1` | Attachment disposition; may extend retention |
| `GET` | `/f/{id}?json=1` | Metadata as JSON |
| `GET` | `/install.sh` | CLI install script (Unix) |
| `GET` | `/install.ps1` | CLI install script (Windows) |

Download behavior:

- Single-chunk files often **302** to Discord CDN.
- Multi-chunk or ranged requests stream through the API.
- Full `?download=1` (non-`HEAD`, no `Range`) extends expiry by +7 days (cap now+30d) and may count toward download limits for non-managers.
- When known, responses set `X-Content-SHA256` and `Digest: sha-256=…`. See [file-integrity.md](file-integrity.md).

## Example: create an upload session with a PAT

```bash
curl -H "Authorization: Bearer dc_…" \
  -H "Content-Type: application/json" \
  -X POST "https://api.example.com/api/uploads" \
  -d '{"fileName":"a.bin","fileSize":123}'
```
