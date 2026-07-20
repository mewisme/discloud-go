import type { Metadata } from "next";

import { DocsCode, DocsOrigin } from "@/components/docs-code";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";

export const metadata: Metadata = {
  title: "API",
  description:
    "HTTP API reference for DisCloud: uploads, chunked uploads, downloads, and file metadata.",
};

const methodVariant: Record<string, string> = {
  GET: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
  HEAD: "bg-sky-500/10 text-sky-600 dark:text-sky-400",
  POST: "bg-amber-500/10 text-amber-600 dark:text-amber-400",
};

function Method({ children }: { children: string }) {
  return (
    <Badge
      variant="secondary"
      className={`font-mono ${methodVariant[children] ?? ""}`}
    >
      {children}
    </Badge>
  );
}

function Path({ children }: { children: string }) {
  return <code className="font-mono text-sm font-semibold">{children}</code>;
}

function Endpoint({
  method,
  path,
  children,
}: {
  method: string;
  path: string;
  children: React.ReactNode;
}) {
  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2">
        <Method>{method}</Method>
        <Path>{path}</Path>
      </div>
      {children}
    </section>
  );
}

function P({ children }: { children: React.ReactNode }) {
  return <p className="text-sm text-muted-foreground">{children}</p>;
}

export default function DocsPage() {
  return (
    <div className="flex flex-col gap-8">
      <header className="flex flex-col gap-2">
        <h1 className="text-2xl font-semibold tracking-tight">API reference</h1>
        <P>
          Everything the web UI does is available over plain HTTP on{" "}
          <strong className="text-foreground">this same origin</strong> — the
          Next.js server rewrites these paths to the storage API, so scripts
          work through the exact domain you are reading this page on (including
          behind Cloudflare). No authentication is required. Examples below use
          your current origin: <DocsOrigin />.
        </P>
        <DocsCode>{`BASE=$BASE`}</DocsCode>
      </header>

      <Separator />

      <h2 className="text-lg font-semibold tracking-tight">Uploading</h2>

      <Endpoint method="POST" path="/api/upload?fileName={name}">
        <P>
          Single-request upload. The raw file bytes are the request body; the
          server splits them into 8 MB chunks internally. Simplest option for
          files under proxy body limits (Cloudflare caps proxied requests at
          100 MB — use the chunked flow below for anything bigger).
        </P>
        <DocsCode>{`curl -X POST --data-binary @video.mp4 "$BASE/api/upload?fileName=video.mp4"`}</DocsCode>
        <P>Response (200):</P>
        <DocsCode>{`{
  "fileId": "894d9eec70b09280134933c50b168592",
  "fileName": "video.mp4",
  "fileSize": 10485760,
  "url": "$BASE/f/894d9eec70b09280134933c50b168592",
  "longURL": "$BASE/f/894d9eec70b09280134933c50b168592/video.mp4",
  "downloadURL": "$BASE/f/894d9eec70b09280134933c50b168592?download=1",
  "longDownloadURL": "$BASE/f/894d9eec70b09280134933c50b168592/video.mp4?download=1"
}`}</DocsCode>
        <P>
          Errors: 400 when <code className="font-mono">fileName</code> is
          missing or the body is empty.
        </P>
      </Endpoint>

      <Separator />

      <h2 className="text-lg font-semibold tracking-tight">
        Chunked uploading (any size, resumable)
      </h2>
      <P>
        Split the file into <strong className="text-foreground">8 MB</strong>{" "}
        chunks (every chunk except the last must be exactly 8&nbsp;MB). Chunks
        are content-addressed by their hex SHA-256: if the server already has a
        chunk, you can skip uploading it — which makes retries resume for free
        and deduplicates identical data across files.
      </P>

      <Endpoint method="GET/HEAD" path="/api/chunks/{sha256}">
        <P>
          Ask whether a chunk is already stored. 200 means you can skip the
          upload, 404 means send it.
        </P>
        <DocsCode>{`curl -I "$BASE/api/chunks/2af4eff90cc2b40f8f852cea020faf54c44f102467096d36b4a40e5fbb3d8eaa"
# HTTP/1.1 200 -> already stored, skip
# HTTP/1.1 404 -> upload it`}</DocsCode>
      </Endpoint>

      <Endpoint method="POST" path="/api/chunks">
        <P>
          Upload one chunk (raw bytes, max 8 MB). The hash is computed
          server-side from the received bytes — use the returned value for the
          complete call.{" "}
          <code className="font-mono">existed: true</code> means the chunk was
          already stored and no new copy was made.
        </P>
        <DocsCode>{`curl -X POST --data-binary @part-aa "$BASE/api/chunks"`}</DocsCode>
        <P>Response (200):</P>
        <DocsCode>{`{ "hash": "2af4eff9…3d8eaa", "existed": false }`}</DocsCode>
        <P>Errors: 400 for an empty body, 413 for a body over 8 MB.</P>
      </Endpoint>

      <Endpoint method="POST" path="/api/upload/complete">
        <P>
          Assemble a file from previously uploaded chunks, in order. The file
          size is computed from the stored chunk sizes.
        </P>
        <DocsCode>{`curl -X POST -H "Content-Type: application/json" "$BASE/api/upload/complete" -d '{
  "fileName": "video.mp4",
  "chunkHashes": ["<hash of chunk 1>", "<hash of chunk 2>"]
}'`}</DocsCode>
        <P>
          Response (200): same shape as{" "}
          <code className="font-mono">/api/upload</code>. Errors: 400 for an
          unknown or malformed hash, a missing{" "}
          <code className="font-mono">fileName</code>, an empty hash list, or a
          non-final chunk that is not exactly 8 MB.
        </P>
        <P>Complete example with bash:</P>
        <DocsCode>{`split -b 8m video.mp4 part-
hashes=()
for p in part-*; do
  hashes+=("$(curl -s -X POST --data-binary @"$p" "$BASE/api/chunks" | jq -r .hash)")
done
printf '%s\\n' "\${hashes[@]}" | jq -R . | jq -s \\
  '{fileName: "video.mp4", chunkHashes: .}' | \\
  curl -s -X POST -H "Content-Type: application/json" -d @- "$BASE/api/upload/complete"`}</DocsCode>
      </Endpoint>

      <Separator />

      <h2 className="text-lg font-semibold tracking-tight">Downloading</h2>

      <Endpoint method="GET" path="/f/{fileId}[/{fileName}]">
        <P>
          Stream a file. The trailing file name segment is optional and purely
          cosmetic. Add <code className="font-mono">?download=1</code> for a{" "}
          <code className="font-mono">Content-Disposition: attachment</code>{" "}
          response. Single-range <code className="font-mono">Range</code>{" "}
          requests are supported (206 with{" "}
          <code className="font-mono">Content-Range</code>); open-ended ranges
          like <code className="font-mono">bytes=0-</code> return at most a
          5 MB window, which is what media players expect when seeking.
        </P>
        <DocsCode>{`curl -OJ "$BASE/f/894d9eec70b09280134933c50b168592/video.mp4?download=1"

# Seek into the middle of a file
curl -H "Range: bytes=8388508-8388708" "$BASE/f/894d9eec70b09280134933c50b168592"`}</DocsCode>
        <P>Errors: 404 for an unknown id, 416 for an unsatisfiable range.</P>
      </Endpoint>

      <Separator />

      <h2 className="text-lg font-semibold tracking-tight">Files & health</h2>

      <Endpoint method="GET" path="/api/files">
        <P>The 50 most recent uploads, newest first.</P>
        <DocsCode>{`{
  "files": [
    {
      "fileId": "894d9eec70b09280134933c50b168592",
      "fileName": "video.mp4",
      "fileSize": 10485760,
      "chunkSize": 8388608,
      "createdAt": "2026-07-20T05:32:41.123Z"
    }
  ]
}`}</DocsCode>
      </Endpoint>

      <Endpoint method="GET" path="/api/files/{fileId}">
        <P>Metadata for one file (same object shape). 404 if unknown.</P>
      </Endpoint>

      <Endpoint method="GET" path="/readyz">
        <P>
          Readiness probe: 200 when PostgreSQL and Valkey are both reachable,
          503 otherwise. The UI polls this to show its degraded-service banner.
        </P>
      </Endpoint>

      <Separator />

      <footer className="flex flex-col gap-2 pb-4">
        <h2 className="text-lg font-semibold tracking-tight">Notes</h2>
        <ul className="list-inside list-disc text-sm text-muted-foreground [&>li]:mt-1">
          <li>
            All error responses are JSON:{" "}
            <code className="font-mono">{`{ "message": "…" }`}</code>.
          </li>
          <li>
            File names are sanitized to kebab-case on the server (
            <code className="font-mono">My File (1).PDF</code> →{" "}
            <code className="font-mono">my-file-1.pdf</code>).
          </li>
          <li>
            Uploaded chunks live as Discord attachments; signed CDN links are
            refreshed automatically on download, so share URLs never expire.
          </li>
        </ul>
      </footer>
    </div>
  );
}
