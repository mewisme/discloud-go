import type { Metadata } from "next";
import type { ReactNode } from "react";

import { DocsCode, DocsOrigin } from "@/components/docs-code";
import { Badge } from "@/components/ui/badge";

export const metadata: Metadata = {
  title: "API",
  description: "HTTP API reference for DisCloud.",
};

const methodVariant: Record<string, string> = {
  GET: "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400",
  HEAD: "bg-sky-500/10 text-sky-700 dark:text-sky-400",
  POST: "bg-amber-500/10 text-amber-700 dark:text-amber-400",
  PATCH: "bg-violet-500/10 text-violet-700 dark:text-violet-400",
  DELETE: "bg-rose-500/10 text-rose-700 dark:text-rose-400",
};

const toc = [
  { id: "base", label: "Base URL" },
  { id: "auth", label: "Auth" },
  { id: "upload", label: "Upload" },
  { id: "chunked", label: "Chunked upload" },
  { id: "download", label: "Download" },
  { id: "files", label: "Files & inspect" },
  { id: "ops", label: "Ops" },
  { id: "notes", label: "Notes" },
] as const;

function Method({ children }: { children: string }) {
  const parts = children.split("/");
  return (
    <span className="inline-flex flex-wrap gap-1">
      {parts.map((m) => (
        <Badge
          key={m}
          variant="secondary"
          className={`font-mono ${methodVariant[m] ?? ""}`}
        >
          {m}
        </Badge>
      ))}
    </span>
  );
}

function Endpoint({
  method,
  path,
  children,
}: {
  method: string;
  path: string;
  children: ReactNode;
}) {
  return (
    <article className="rounded-xl border border-border/60 bg-card/40 p-4 sm:p-5">
      <div className="mb-3 flex flex-wrap items-center gap-2">
        <Method>{method}</Method>
        <code className="font-mono text-sm font-semibold break-all">{path}</code>
      </div>
      <div className="flex flex-col gap-3 text-sm text-muted-foreground [&_strong]:font-medium [&_strong]:text-foreground [&_code]:rounded [&_code]:bg-muted/60 [&_code]:px-1 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.8rem] [&_code]:text-foreground">
        {children}
      </div>
    </article>
  );
}

function Label({ children }: { children: ReactNode }) {
  return (
    <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
      {children}
    </p>
  );
}

export default function DocsPage() {
  return (
    <div className="flex flex-col gap-10 lg:flex-row lg:gap-12">
      <nav
        aria-label="On this page"
        className="lg:sticky lg:top-20 lg:h-fit lg:w-44 lg:shrink-0"
      >
        <p className="mb-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          On this page
        </p>
        <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm lg:flex-col lg:gap-1.5">
          {toc.map((item) => (
            <li key={item.id}>
              <a
                href={`#${item.id}`}
                className="text-muted-foreground transition-colors hover:text-foreground"
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>

      <div className="min-w-0 flex-1 space-y-10">
        <header className="space-y-2">
          <h1 className="text-2xl font-semibold tracking-tight">API reference</h1>
          <p className="max-w-2xl text-sm text-muted-foreground">
            Plain HTTP. Optional email/password sessions (
            <code className="font-mono text-foreground">discloud_session</code>
            ). Browser clients must send credentials and match{" "}
            <code className="font-mono text-foreground">WEB_ORIGIN</code>.
          </p>
        </header>

        <section id="base" className="scroll-mt-24 space-y-3">
          <h2 className="text-lg font-semibold tracking-tight">Base URL</h2>
          <p className="text-sm text-muted-foreground">
            API origin (from <code className="font-mono text-foreground">API_URL</code>
            ): <DocsOrigin />
          </p>
          <DocsCode>{`export BASE=$BASE`}</DocsCode>
        </section>

        <section id="auth" className="scroll-mt-24 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">Auth</h2>
            <p className="text-sm text-muted-foreground">
              Cookie session. First signup on a fresh DB becomes{" "}
              <code>admin</code>; later accounts are <code>user</code>. Password
              min 8 chars.
            </p>
          </div>

          <Endpoint method="POST" path="/api/auth/signup">
            <DocsCode>{`curl -X POST -H "Content-Type: application/json" -H "Origin: http://localhost:3000" \\
  -c cookies.txt -d '{"email":"you@example.com","password":"secret123"}' \\
  "$BASE/api/auth/signup"
# → { "id", "email", "role" } + Set-Cookie: discloud_session=…`}</DocsCode>
          </Endpoint>

          <Endpoint method="POST" path="/api/auth/signin">
            <p>Same body as signup. Invalid credentials → 401.</p>
            <DocsCode>{`curl -X POST -H "Content-Type: application/json" -H "Origin: http://localhost:3000" \\
  -c cookies.txt -d '{"email":"you@example.com","password":"secret123"}' \\
  "$BASE/api/auth/signin"`}</DocsCode>
          </Endpoint>

          <Endpoint method="POST" path="/api/auth/signout">
            <p>Clears the session cookie. 204.</p>
            <DocsCode>{`curl -X POST -H "Origin: http://localhost:3000" -b cookies.txt \\
  "$BASE/api/auth/signout"`}</DocsCode>
          </Endpoint>

          <Endpoint method="GET" path="/api/auth/me">
            <p>Current user, or 401.</p>
            <DocsCode>{`curl -s -b cookies.txt "$BASE/api/auth/me"`}</DocsCode>
          </Endpoint>
        </section>

        <section id="upload" className="scroll-mt-24 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">Upload</h2>
            <p className="text-sm text-muted-foreground">
              One request for small/medium files. Body = raw bytes. Server splits
              into 8&nbsp;MB chunks. Public by default. With a session cookie,
              the file is owned (30-day retention); anonymous = 7 days.
            </p>
          </div>

          <Endpoint method="POST" path="/api/upload?fileName={name}">
            <p>
              Simplest path when proxies allow the whole body (e.g. Cloudflare
              caps at ~100&nbsp;MB — use chunked upload for larger files).
            </p>
            <Label>Example</Label>
            <DocsCode>{`curl -X POST --data-binary @video.mp4 \\
  "$BASE/api/upload?fileName=video.mp4"`}</DocsCode>
            <Label>Response · 200</Label>
            <DocsCode>{`{
  "fileId": "894d9eec70b09280134933c50b168592",
  "fileName": "video.mp4",
  "fileSize": 10485760,
  "chunkSize": 8388608,
  "visibility": "public",
  "ownedByCurrentUser": false,
  "createdAt": "…",
  "expiresAt": "…",
  "url": "$BASE/f/894d9eec…",
  "longURL": "$BASE/f/894d9eec…/video.mp4",
  "downloadURL": "$BASE/f/894d9eec…?download=1",
  "longDownloadURL": "$BASE/f/894d9eec…/video.mp4?download=1"
}`}</DocsCode>
            <p>
              <strong>Errors:</strong> 400 if <code>fileName</code> is missing or
              the body is empty.
            </p>
          </Endpoint>
        </section>

        <section id="chunked" className="scroll-mt-24 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">
              Chunked upload
            </h2>
            <p className="text-sm text-muted-foreground">
              Any size, resumable. Split into <strong>8&nbsp;MB</strong> pieces
              (last chunk may be shorter). Each chunk is keyed by SHA-256 — skip
              uploads the server already has. Send credentials on complete so
              ownership attaches when signed in.
            </p>
          </div>

          <Endpoint method="GET/HEAD" path="/api/chunks/{sha256}">
            <p>
              Check if a chunk exists. <strong>200</strong> = skip,{" "}
              <strong>404</strong> = upload it.
            </p>
            <DocsCode>{`curl -I "$BASE/api/chunks/<sha256>"`}</DocsCode>
          </Endpoint>

          <Endpoint method="POST" path="/api/chunks">
            <p>
              Upload one chunk (raw body, max 8&nbsp;MB). Hash is computed
              server-side. <code>existed: true</code> means it was already stored.
            </p>
            <DocsCode>{`curl -X POST --data-binary @part-aa "$BASE/api/chunks"
# → { "hash": "2af4eff9…", "existed": false }`}</DocsCode>
            <p>
              <strong>Errors:</strong> 400 empty body · 413 over 8&nbsp;MB.
            </p>
          </Endpoint>

          <Endpoint method="POST" path="/api/upload/complete">
            <p>Assemble chunks in order. Response matches single upload.</p>
            <DocsCode>{`curl -X POST -H "Content-Type: application/json" \\
  "$BASE/api/upload/complete" \\
  -d '{
    "fileName": "video.mp4",
    "chunkHashes": ["<hash1>", "<hash2>"]
  }'`}</DocsCode>
            <Label>Bash · split + upload + complete</Label>
            <DocsCode>{`split -b 8m video.mp4 part-
hashes=()
for p in part-*; do
  hashes+=("$(curl -s -X POST --data-binary @"$p" \\
    "$BASE/api/chunks" | jq -r .hash)")
done
printf '%s\\n' "\${hashes[@]}" | jq -R . | jq -s \\
  '{fileName: "video.mp4", chunkHashes: .}' |
  curl -s -X POST -H "Content-Type: application/json" \\
    -d @- "$BASE/api/upload/complete"`}</DocsCode>
          </Endpoint>
        </section>

        <section id="download" className="scroll-mt-24 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">Download</h2>
            <p className="text-sm text-muted-foreground">
              Stream bytes, force download, or fetch metadata. Single-chunk
              responses may redirect to the CDN. Private files need a session
              (owner/admin) or <code>?token=</code> / <code>X-File-Token</code>.
            </p>
          </div>

          <Endpoint method="GET" path="/f/{fileId}[/{fileName}]">
            <ul className="list-disc space-y-1 pl-4">
              <li>
                Optional <code>{"/{fileName}"}</code> is cosmetic (nice share
                URLs).
              </li>
              <li>
                <code>?download=1</code> → attachment disposition; extends
                retention (not HEAD, not Range)
              </li>
              <li>
                <code>?json=1</code> → metadata JSON (no file bytes)
              </li>
              <li>
                <code>?token=</code> → private file access token
              </li>
              <li>
                <code>Range</code> → 206; open-ended ranges capped at 5&nbsp;MB
              </li>
            </ul>
            <DocsCode>{`curl -OJ "$BASE/f/<id>/video.mp4?download=1"
curl -s "$BASE/f/<id>?json=1"
curl -H "Range: bytes=0-1023" "$BASE/f/<id>"
curl -s "$BASE/f/<id>?token=<accessToken>&json=1"`}</DocsCode>
            <p>
              <strong>Errors:</strong> 404 unknown / expired / unauthorized
              private · 416 bad range.
            </p>
          </Endpoint>
        </section>

        <section id="files" className="scroll-mt-24 space-y-4">
          <div className="space-y-1">
            <h2 className="text-lg font-semibold tracking-tight">
              Files & inspect
            </h2>
          </div>

          <Endpoint method="GET" path="/api/files">
            <p>
              <strong>Auth required.</strong> Files owned by the current user
              (paginated <code>limit</code>/<code>offset</code>).
            </p>
            <DocsCode>{`curl -s -b cookies.txt "$BASE/api/files"
# { "files": [{ "fileId", "fileName", "visibility", "expiresAt", … }] }`}</DocsCode>
          </Endpoint>

          <Endpoint method="GET" path="/api/files/{fileId}">
            <p>
              One file’s metadata. Private: owner/admin session or token. 404 if
              unknown / denied / expired.
            </p>
            <DocsCode>{`curl -s "$BASE/api/files/<id>"
curl -s "$BASE/api/files/<id>?token=<accessToken>"`}</DocsCode>
          </Endpoint>

          <Endpoint method="GET" path="/api/files/{fileId}/inspect">
            <p>
              Analytics (views, downloads, ranges, bytes served, unique visitors)
              plus share URLs. Same authz as metadata. UI:{" "}
              <code>/i/{"{fileId}"}</code>.
            </p>
            <DocsCode>{`curl -s "$BASE/api/files/<id>/inspect"`}</DocsCode>
          </Endpoint>

          <Endpoint method="PATCH" path="/api/files/{fileId}/visibility">
            <p>
              Owner or admin. Body:{" "}
              <code>{`{ "visibility": "public" | "private" }`}</code>. Private
              returns a one-time <code>accessToken</code> (shown once — rotate to
              recover). Anonymous-owned files cannot go private (403). Public
              clears the token.
            </p>
            <DocsCode>{`curl -X PATCH -H "Content-Type: application/json" -H "Origin: http://localhost:3000" \\
  -b cookies.txt -d '{"visibility":"private"}' \\
  "$BASE/api/files/<id>/visibility"
# → { "visibility": "private", "accessToken": "…" }`}</DocsCode>
          </Endpoint>

          <Endpoint method="POST" path="/api/files/{fileId}/access-token/rotate">
            <p>
              Owner or admin; private files only. Returns a new one-time{" "}
              <code>accessToken</code>; old token stops working.
            </p>
            <DocsCode>{`curl -X POST -H "Origin: http://localhost:3000" -b cookies.txt \\
  "$BASE/api/files/<id>/access-token/rotate"`}</DocsCode>
          </Endpoint>

          <Endpoint method="DELETE" path="/api/files/{fileId}">
            <p>
              Owner or admin. <strong>204</strong>. Deletes Postgres metadata
              only — Discord attachments are untouched. File tokens cannot
              delete.
            </p>
            <DocsCode>{`curl -X DELETE -H "Origin: http://localhost:3000" -b cookies.txt \\
  "$BASE/api/files/<id>"`}</DocsCode>
          </Endpoint>

          <Endpoint method="GET" path="/api/info">
            <p>
              Public config for clients: bot/worker hint and chunk size.
            </p>
            <DocsCode>{`curl -s "$BASE/api/info"
# { "bots": 1, "chunkSize": 8388608, "workers": 3 }`}</DocsCode>
          </Endpoint>
        </section>

        <section id="ops" className="scroll-mt-24 space-y-4">
          <h2 className="text-lg font-semibold tracking-tight">Ops</h2>

          <Endpoint method="GET" path="/healthz">
            <p>Liveness. Always 200 if the process is up.</p>
            <DocsCode>{`curl -s "$BASE/healthz"   # ok`}</DocsCode>
          </Endpoint>

          <Endpoint method="GET" path="/readyz">
            <p>
              Readiness: 200 when Postgres + Valkey are reachable, else 503. The
              UI polls this for the degraded banner.
            </p>
            <DocsCode>{`curl -s -o /dev/null -w "%{http_code}\\n" "$BASE/readyz"`}</DocsCode>
          </Endpoint>
        </section>

        <section id="notes" className="scroll-mt-24 space-y-3 pb-6">
          <h2 className="text-lg font-semibold tracking-tight">Notes</h2>
          <ul className="list-disc space-y-2 pl-5 text-sm text-muted-foreground">
            <li>
              Errors are JSON:{" "}
              <code className="font-mono text-foreground">{`{ "message": "…" }`}</code>
            </li>
            <li>
              File names are kebab-cased (
              <code className="font-mono text-foreground">My File (1).PDF</code>{" "}
              → <code className="font-mono text-foreground">my-file-1.pdf</code>
              )
            </li>
            <li>
              CDN URLs are refreshed on download so share links stay valid
            </li>
            <li>
              CORS allowlists exact <code className="font-mono text-foreground">WEB_ORIGIN</code>{" "}
              with credentials. Mutating requests with a session cookie (or with
              an <code>Origin</code> header) must match that origin.
            </li>
            <li>
              <code className="font-mono text-foreground">SameSite=Lax</code> cookies
              need API + UI same-site (path proxy in prod; localhost ports OK).
            </li>
            <li>
              Retention: anonymous 7d, authenticated 30d;{" "}
              <code>?download=1</code> extends by 7d (cap 30d from now). Cleanup
              deletes expired Postgres rows only.
            </li>
            <li>
              Private denials are uniform <strong>404</strong> (no existence leak).
              Token-authenticated responses set{" "}
              <code>Referrer-Policy: no-referrer</code>.
            </li>
          </ul>
        </section>
      </div>
    </div>
  );
}
