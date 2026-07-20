export interface UploadResult {
  fileId: string;
  fileName: string;
  fileSize: number;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
}

/** GET /api/files/{id} */
export interface FileMeta {
  fileId: string;
  fileName: string;
  fileSize: number;
  chunkSize: number;
  createdAt: string;
}

/** Absolute URL on the public API (no Next proxy). */
export function apiURL(path: string): string {
  const injected =
    typeof globalThis !== "undefined"
      ? (globalThis as { __DISCLOUD_API__?: string }).__DISCLOUD_API__
      : undefined;
  const base = (injected || "http://localhost:8080").replace(/\/$/, "");
  const p = path.startsWith("/") ? path : `/${path}`;
  return `${base}${p}`;
}

export async function fetchFileMeta(
  fileId: string,
  init?: RequestInit,
): Promise<FileMeta> {
  const res = await fetch(apiURL(`/api/files/${fileId}`), {
    cache: "no-store",
    ...init,
  });
  if (res.status === 404) throw new Error("File not found on server");
  if (!res.ok) throw new Error(`Could not load metadata (${res.status})`);
  return (await res.json()) as FileMeta;
}
