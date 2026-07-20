import type { UploadResult } from "@/lib/api";

/**
 * Chunked upload that stays under proxy body-size limits (Cloudflare caps
 * proxied requests at 100 MB): the file is split into 8 MB chunks matching
 * the server's storage chunk size, each chunk is SHA-256 hashed and skipped
 * if the server already has it, so retried uploads resume where they left off.
 *
 * Worker count comes from GET /api/info (matches Discord bot token count when
 * multiple tokens are configured).
 */
const CHUNK_SIZE = 8 * 1024 * 1024;
const DEFAULT_WORKERS = 3;
const ATTEMPTS = 3;

let workersPromise: Promise<number> | null = null;

/** Parallel chunk POSTs — scales with Discord bot tokens on the server. */
export async function uploadWorkers(): Promise<number> {
  if (!workersPromise) {
    workersPromise = fetch("/api/info", { cache: "no-store" })
      .then(async (res) => {
        if (!res.ok) return DEFAULT_WORKERS;
        const body = (await res.json()) as { workers?: unknown };
        const n = Number(body.workers);
        return Number.isFinite(n) && n >= 1 ? Math.min(Math.floor(n), 32) : DEFAULT_WORKERS;
      })
      .catch(() => DEFAULT_WORKERS);
  }
  return workersPromise;
}

export async function uploadFileChunked(
  file: File,
  onProgress: (loaded: number, total: number) => void,
): Promise<UploadResult> {
  if (file.size === 0) throw new Error("File is empty");
  if (!crypto.subtle) {
    throw new Error("Uploads require a secure context (https or localhost)");
  }

  const chunkCount = Math.ceil(file.size / CHUNK_SIZE);
  const hashes: string[] = new Array(chunkCount);
  const loaded: number[] = new Array(chunkCount).fill(0);
  const report = () =>
    onProgress(loaded.reduce((a, b) => a + b, 0), file.size);

  const workers = Math.min(await uploadWorkers(), chunkCount);
  let next = 0;
  async function worker(): Promise<void> {
    while (next < chunkCount) {
      const idx = next++;
      const blob = file.slice(idx * CHUNK_SIZE, Math.min((idx + 1) * CHUNK_SIZE, file.size));
      hashes[idx] = await uploadChunkWithRetry(blob, (sent) => {
        loaded[idx] = sent;
        report();
      });
      loaded[idx] = blob.size;
      report();
    }
  }
  await Promise.all(Array.from({ length: workers }, worker));

  const res = await fetch("/api/upload/complete", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fileName: file.name, chunkHashes: hashes }),
  });
  if (!res.ok) {
    throw new Error(`Could not finalize upload (${res.status})`);
  }
  return (await res.json()) as UploadResult;
}

async function uploadChunkWithRetry(
  blob: Blob,
  onChunkProgress: (sent: number) => void,
): Promise<string> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= ATTEMPTS; attempt++) {
    try {
      const buf = await blob.arrayBuffer();
      const hash = await sha256Hex(buf);
      // Skip the upload entirely if the server already has these bytes.
      const check = await fetch(`/api/chunks/${hash}`, { method: "HEAD" });
      if (check.ok) return hash;
      return await postChunk(buf, onChunkProgress);
    } catch (err) {
      lastError = err;
      onChunkProgress(0);
      if (attempt < ATTEMPTS) {
        await new Promise((r) => setTimeout(r, 500 * attempt));
      }
    }
  }
  throw lastError instanceof Error ? lastError : new Error("Chunk upload failed");
}

/** XHR instead of fetch purely for upload progress events. */
function postChunk(
  buf: ArrayBuffer,
  onChunkProgress: (sent: number) => void,
): Promise<string> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/chunks");
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onChunkProgress(e.loaded);
    };
    xhr.onload = () => {
      if (xhr.status === 200) {
        const body = JSON.parse(xhr.responseText) as { hash: string };
        resolve(body.hash);
      } else {
        reject(new Error(`Chunk upload failed (${xhr.status})`));
      }
    };
    xhr.onerror = () => reject(new Error("Chunk upload failed: network error"));
    xhr.send(buf);
  });
}

async function sha256Hex(buf: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", buf);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
