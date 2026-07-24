# Security

Report vulnerabilities privately — see [SECURITY.md](../SECURITY.md).

## Authentication models

| Concept | Meaning |
| --- | --- |
| **Session cookie** | `discloud_session` after signup/signin. Full access for that user; no PAT scope checks. |
| **API token** | Personal access token `dc_…` via `Authorization: Bearer`. Scopes gate each route. |
| **Admin role** | `users.role = admin`. First signup on an empty DB becomes admin (advisory lock). |
| **Admin token scope** | PAT scope `admin`. Required for Bearer calls to admin APIs **in addition to** admin role. Cookie admin sessions do not need the scope. |

Auth resolution order on the API: cookie session first, then Bearer token.

### Passwords

- Account passwords and file-share passwords use **Argon2id**.
- Server stores hashes only.
- Share unlock for browsers: `POST /api/files/{id}/unlock` sets an unlock cookie (`discloud_unlock_{fileId}`, ~1 hour).
- Non-browser clients send `X-File-Password`.

### CSRF and cookies

- Browser credentialed requests must send `Origin` matching `DISCLOUD_CORS` exactly.
- When `DISCLOUD_CORS` is `https://…`, the session cookie is marked **Secure**.
- CLI must set `--origin` / `DISCLOUD_ORIGIN` to the same value when using cookie login.

### Auth rate limiting

- Signup/signin (and related auth attempts): **10 attempts / 15 minutes** per key in Valkey.
- Upload and download paths have separate per-minute limits (see [configuration.md](configuration.md)).

## Authorization for files

| Gate | Behavior |
| --- | --- |
| Public visibility | Readable without ownership |
| Private visibility | Owner/admin session, or valid access token (`?token=` / `X-File-Token`) |
| Password share | After visibility/token; managers skip password |
| Revoke | Force-expires the file and clears password settings |
| Delete | Removes Postgres metadata only |

Visibility/token denials are uniform **404**. Password failures are **401** (`password_required` / `password_invalid`).

## Secrets

Keep out of git and logs:

- `DISCLOUD_BOT_TOKEN`
- Database credentials
- `DISCLOUD_APP_SECRET` / `.app.secret`
- `.visitor.secret` (visitor hashing salt)
- Raw API tokens (`dc_…`)

Persist `/data` in Docker so auto-generated secrets survive restarts. Changing `DISCLOUD_APP_SECRET` invalidates existing session cookies, file access tokens, upload resume tokens, and other HMAC-derived material. Multiple API replicas must share the **same** durable secret. See [operations.md](operations.md).

## Discord data

Delete, cancel, revoke, and expiry cleanup **do not** remove Discord attachments. Treat Discord as durable blob storage outside your control. Do not store secrets in uploaded files if Discord retention is unacceptable for your threat model.

## Optional CAPTCHA

When `DISCLOUD_CAPTCHA_SECRET` is set on the API, anonymous upload-session create requires Cloudflare Turnstile. The web UI reads the site key from `DISCLOUD_CAPTCHA_KEY` via its `/api/captcha` route.
