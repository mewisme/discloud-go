import type { UploadResult } from "@/lib/api";

/**
 * Chunked upload under proxy body-size limits: split into 8 MB pieces, skip
 * chunks the server already has, upload missing ones in Discord-sized batches
 * (up to 10 attachments per POST /api/chunks/batch).
 */
const CHUNK_SIZE = 8 * 1024 * 1024;
const BATCH_SIZE = 10;
const DEFAULT_WORKERS = 3;
const ATTEMPTS = 3;

let workersPromise: Promise<number> | null = null;

/** Parallel batch POSTs — scales with Discord bot tokens on the server. */
export async function uploadWorkers(): Promise<number> {
  if (!workersPromise) {
    workersPromise = fetch("/api/info", { cache: "no-store" })
      .then(async (res) => {
        if (!res.ok) return DEFAULT_WORKERS;
        const body = (await res.json()) as { workers?: unknown };
        const n = Number(body.workers);
        return Number.isFinite(n) && n >= 1
          ? Math.min(Math.floor(n), 32)
          : DEFAULT_WORKERS;
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
  const missing: { idx: number; buf: ArrayBuffer; hash: string }[] = [];

  const workers = Math.min(await uploadWorkers(), chunkCount);
  let next = 0;
  const presentBytes = new Array(chunkCount).fill(0);

  async function hashWorker(): Promise<void> {
    while (next < chunkCount) {
      const idx = next++;
      const blob = file.slice(
        idx * CHUNK_SIZE,
        Math.min((idx + 1) * CHUNK_SIZE, file.size),
      );
      const buf = await blob.arrayBuffer();
      const hash = await sha256Hex(buf);
      hashes[idx] = hash;
      const check = await fetch(`/api/chunks/${hash}`, { method: "HEAD" });
      if (!check.ok) {
        missing.push({ idx, buf, hash });
      } else {
        presentBytes[idx] = buf.byteLength;
        onProgress(
          presentBytes.reduce((a: number, b: number) => a + b, 0),
          file.size,
        );
      }
    }
  }
  await Promise.all(Array.from({ length: workers }, hashWorker));

  // Stable order for complete; upload missing in batches of ≤10.
  missing.sort((a, b) => a.idx - b.idx);
  const batches: (typeof missing)[] = [];
  for (let i = 0; i < missing.length; i += BATCH_SIZE) {
    batches.push(missing.slice(i, i + BATCH_SIZE));
  }

  const presentTotal = presentBytes.reduce((a: number, b: number) => a + b, 0);
  const loadedExtra = new Array(batches.length).fill(0);
  const report = () =>
    onProgress(
      presentTotal + loadedExtra.reduce((a: number, b: number) => a + b, 0),
      file.size,
    );

  let batchNext = 0;
  async function batchWorker(): Promise<void> {
    while (batchNext < batches.length) {
      const bi = batchNext++;
      const batch = batches[bi];
      await uploadBatchWithRetry(batch, (sent) => {
        loadedExtra[bi] = sent;
        report();
      });
      loadedExtra[bi] = batch.reduce((a, c) => a + c.buf.byteLength, 0);
      report();
    }
  }
  await Promise.all(
    Array.from(
      { length: Math.min(workers, Math.max(batches.length, 1)) },
      batchWorker,
    ),
  );

  onProgress(file.size, file.size);

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

async function uploadBatchWithRetry(
  batch: { idx: number; buf: ArrayBuffer; hash: string }[],
  onProgress: (sent: number) => void,
): Promise<void> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= ATTEMPTS; attempt++) {
    try {
      await postChunkBatch(
        batch.map((c) => c.buf),
        onProgress,
      );
      return;
    } catch (err) {
      lastError = err;
      onProgress(0);
      if (attempt < ATTEMPTS) {
        await new Promise((r) => setTimeout(r, 500 * attempt));
      }
    }
  }
  throw lastError instanceof Error ? lastError : new Error("Chunk batch failed");
}

function postChunkBatch(
  bufs: ArrayBuffer[],
  onProgress: (sent: number) => void,
): Promise<void> {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    bufs.forEach((buf, i) => {
      form.append(`files[${i}]`, new Blob([buf]), `chunk-${i}.bin`);
    });
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api/chunks/batch");
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(e.loaded);
    };
    xhr.onload = () => {
      if (xhr.status === 200) resolve();
      else reject(new Error(`Chunk batch failed (${xhr.status})`));
    };
    xhr.onerror = () => reject(new Error("Chunk batch failed: network error"));
    xhr.send(form);
  });
}

async function sha256Hex(buf: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", buf);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
