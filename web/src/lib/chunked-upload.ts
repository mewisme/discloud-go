import { apiURL, type UploadResult } from "@/lib/api";
import {
  clearUploadRecord,
  findUploadRecord,
  saveUploadRecord,
} from "@/lib/upload-session-store";

/**
 * Chunked upload via server upload sessions when available.
 * Falls back to legacy /api/upload/complete.
 */
const CHUNK_SIZE = 8 * 1024 * 1024;
const DEFAULT_WORKERS = 3;
const ATTEMPTS = 3;

export async function uploadWorkers(): Promise<number> {
  return DEFAULT_WORKERS;
}

export type UploadFileOptions = {
  relativePath?: string;
  onProgress: (loaded: number, total: number) => void;
  signal?: AbortSignal;
  onProcessing?: () => void;
  onSession?: (uploadId: string, resumeToken: string) => void;
};

export async function uploadFileChunked(
  file: File,
  onProgressOrOpts:
    | ((loaded: number, total: number) => void)
    | UploadFileOptions,
  signal?: AbortSignal,
  onProcessing?: () => void,
): Promise<UploadResult> {
  const opts: UploadFileOptions =
    typeof onProgressOrOpts === "function"
      ? { onProgress: onProgressOrOpts, signal, onProcessing }
      : onProgressOrOpts;

  if (file.size === 0) throw new Error("File is empty");
  if (!crypto.subtle) {
    throw new Error("Uploads require a secure context (https or localhost)");
  }
  throwIfAborted(opts.signal);

  const fileName =
    opts.relativePath?.replace(/\\/g, "/").replace(/^\/+/, "") ||
    (typeof file.webkitRelativePath === "string" && file.webkitRelativePath
      ? file.webkitRelativePath.replace(/\\/g, "/")
      : file.name);

  const sessions = await serverSupportsSessions(opts.signal);
  if (!sessions) {
    return legacyUpload(file, fileName, opts);
  }
  return sessionUpload(file, fileName, opts);
}

async function serverSupportsSessions(signal?: AbortSignal): Promise<boolean> {
  try {
    const res = await fetch(apiURL("/api/info"), {
      credentials: "include",
      signal,
    });
    if (!res.ok) return false;
    const body = (await res.json()) as {
      uploads?: { sessions?: boolean };
    };
    return body.uploads?.sessions === true;
  } catch {
    return false;
  }
}

async function sessionUpload(
  file: File,
  fileName: string,
  opts: UploadFileOptions,
): Promise<UploadResult> {
  const chunkCount = Math.ceil(file.size / CHUNK_SIZE);
  const fingerprint = `${fileName}|${file.size}|${file.lastModified}`;

  let uploadId = "";
  let resumeToken = "";
  let skip = new Set<number>();

  const existing = await findUploadRecord(fingerprint);
  if (existing) {
    const prog = await getUpload(
      existing.uploadId,
      existing.resumeToken,
      opts.signal,
    );
    if (prog && (prog.status === "pending" || prog.status === "uploading")) {
      uploadId = existing.uploadId;
      resumeToken = existing.resumeToken;
      skip = new Set(
        Object.keys(prog.parts ?? {})
          .map((k) => Number(k))
          .filter((n) => !Number.isNaN(n)),
      );
      for (const idx of prog.unknownIndices ?? []) {
        skip.delete(idx);
      }
    } else {
      await clearUploadRecord(fingerprint);
    }
  }

  if (!uploadId) {
    const created = await createUpload(
      fileName,
      file.size,
      fingerprint,
      opts.signal,
    );
    uploadId = created.uploadId;
    resumeToken = created.resumeToken;
    await saveUploadRecord({
      fingerprint,
      uploadId,
      resumeToken,
      fileName,
      fileSize: file.size,
      lastModified: file.lastModified,
      chunkCount,
    });
  }
  opts.onSession?.(uploadId, resumeToken);

  const hashes: string[] = new Array(chunkCount);
  const loaded: number[] = new Array(chunkCount).fill(0);
  const report = () =>
    opts.onProgress(loaded.reduce((a, b) => a + b, 0), file.size);

  for (const idx of skip) {
    const start = idx * CHUNK_SIZE;
    const end = Math.min(start + CHUNK_SIZE, file.size);
    loaded[idx] = end - start;
  }
  report();

  const workers = Math.min(await uploadWorkers(), chunkCount);
  let next = 0;
  async function worker(): Promise<void> {
    while (next < chunkCount) {
      throwIfAborted(opts.signal);
      const idx = next++;
      if (skip.has(idx)) continue;
      const blob = file.slice(
        idx * CHUNK_SIZE,
        Math.min((idx + 1) * CHUNK_SIZE, file.size),
      );
      const hash = await uploadChunkWithRetry(
        blob,
        (sent) => {
          loaded[idx] = sent;
          report();
        },
        opts.signal,
      );
      hashes[idx] = hash;
      loaded[idx] = blob.size;
      report();
      await registerPart(uploadId, resumeToken, idx, hash, opts.signal);
    }
  }
  await Promise.all(Array.from({ length: workers }, () => worker()));

  throwIfAborted(opts.signal);
  opts.onProcessing?.();
  const res = await fetch(apiURL(`/api/uploads/${uploadId}/complete`), {
    method: "POST",
    credentials: "include",
    headers: { "X-Upload-Token": resumeToken },
    signal: opts.signal,
  });
  if (!res.ok) {
    throw new Error(`Could not finalize upload (${res.status})`);
  }
  await clearUploadRecord(fingerprint);
  return (await res.json()) as UploadResult;
}

async function legacyUpload(
  file: File,
  fileName: string,
  opts: UploadFileOptions,
): Promise<UploadResult> {
  const chunkCount = Math.ceil(file.size / CHUNK_SIZE);
  const hashes: string[] = new Array(chunkCount);
  const loaded: number[] = new Array(chunkCount).fill(0);
  const report = () =>
    opts.onProgress(loaded.reduce((a, b) => a + b, 0), file.size);

  const workers = Math.min(await uploadWorkers(), chunkCount);
  let next = 0;
  async function worker(): Promise<void> {
    while (next < chunkCount) {
      throwIfAborted(opts.signal);
      const idx = next++;
      const blob = file.slice(
        idx * CHUNK_SIZE,
        Math.min((idx + 1) * CHUNK_SIZE, file.size),
      );
      hashes[idx] = await uploadChunkWithRetry(
        blob,
        (sent) => {
          loaded[idx] = sent;
          report();
        },
        opts.signal,
      );
      loaded[idx] = blob.size;
      report();
    }
  }
  await Promise.all(Array.from({ length: workers }, () => worker()));

  throwIfAborted(opts.signal);
  opts.onProcessing?.();
  const res = await fetch(apiURL("/api/upload/complete"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fileName, chunkHashes: hashes }),
    credentials: "include",
    signal: opts.signal,
  });
  if (!res.ok) {
    throw new Error(`Could not finalize upload (${res.status})`);
  }
  return (await res.json()) as UploadResult;
}

async function createUpload(
  fileName: string,
  fileSize: number,
  clientFingerprint: string,
  signal?: AbortSignal,
): Promise<{ uploadId: string; resumeToken: string }> {
  const res = await fetch(apiURL("/api/uploads"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fileName, fileSize, clientFingerprint }),
    credentials: "include",
    signal,
  });
  if (!res.ok) {
    throw new Error(`Could not start upload (${res.status})`);
  }
  return (await res.json()) as { uploadId: string; resumeToken: string };
}

async function getUpload(
  uploadId: string,
  resumeToken: string,
  signal?: AbortSignal,
): Promise<{
  status: string;
  parts?: Record<string, string>;
  unknownIndices?: number[];
} | null> {
  const res = await fetch(apiURL(`/api/uploads/${uploadId}`), {
    credentials: "include",
    headers: { "X-Upload-Token": resumeToken },
    signal,
  });
  if (!res.ok) return null;
  return (await res.json()) as {
    status: string;
    parts?: Record<string, string>;
    unknownIndices?: number[];
  };
}

async function registerPart(
  uploadId: string,
  resumeToken: string,
  idx: number,
  hash: string,
  signal?: AbortSignal,
): Promise<void> {
  const res = await fetch(apiURL(`/api/uploads/${uploadId}/parts/${idx}`), {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
      "X-Upload-Token": resumeToken,
    },
    body: JSON.stringify({ hash }),
    credentials: "include",
    signal,
  });
  if (!res.ok) {
    throw new Error(`Could not register part ${idx} (${res.status})`);
  }
}

export async function cancelUploadSession(
  uploadId: string,
  resumeToken: string,
): Promise<void> {
  try {
    await fetch(apiURL(`/api/uploads/${uploadId}`), {
      method: "DELETE",
      credentials: "include",
      headers: { "X-Upload-Token": resumeToken },
    });
  } catch {
    /* ignore */
  }
}

async function uploadChunkWithRetry(
  blob: Blob,
  onChunkProgress: (sent: number) => void,
  signal?: AbortSignal,
): Promise<string> {
  let lastError: unknown;
  for (let attempt = 1; attempt <= ATTEMPTS; attempt++) {
    throwIfAborted(signal);
    try {
      const buf = await blob.arrayBuffer();
      const hash = await sha256Hex(buf);
      const check = await fetch(apiURL(`/api/chunks/${hash}`), {
        method: "HEAD",
        credentials: "include",
        signal,
      });
      if (check.ok) return hash;
      return await postChunk(buf, onChunkProgress, signal);
    } catch (err) {
      if (isAbortError(err)) throw err;
      lastError = err;
      onChunkProgress(0);
      if (attempt < ATTEMPTS) {
        await new Promise((r) => setTimeout(r, 500 * attempt));
      }
    }
  }
  throw lastError instanceof Error
    ? lastError
    : new Error("Chunk upload failed");
}

function postChunk(
  buf: ArrayBuffer,
  onChunkProgress: (sent: number) => void,
  signal?: AbortSignal,
): Promise<string> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(abortError());
      return;
    }
    const xhr = new XMLHttpRequest();
    const onAbort = () => {
      xhr.abort();
      reject(abortError());
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    xhr.open("POST", apiURL("/api/chunks"));
    xhr.withCredentials = true;
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onChunkProgress(e.loaded);
    };
    xhr.onload = () => {
      signal?.removeEventListener("abort", onAbort);
      if (xhr.status === 200) {
        const body = JSON.parse(xhr.responseText) as { hash: string };
        resolve(body.hash);
      } else {
        reject(new Error(`Chunk upload failed (${xhr.status})`));
      }
    };
    xhr.onerror = () => {
      signal?.removeEventListener("abort", onAbort);
      reject(new Error("Chunk upload failed: network error"));
    };
    xhr.onabort = () => {
      signal?.removeEventListener("abort", onAbort);
      reject(abortError());
    };
    xhr.send(buf);
  });
}

function throwIfAborted(signal?: AbortSignal): void {
  if (signal?.aborted) throw abortError();
}

function abortError(): DOMException {
  return new DOMException("Upload cancelled", "AbortError");
}

export function isAbortError(err: unknown): boolean {
  return (
    (err instanceof DOMException && err.name === "AbortError") ||
    (err instanceof Error && err.name === "AbortError")
  );
}

async function sha256Hex(buf: ArrayBuffer): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", buf);
  return Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
}
