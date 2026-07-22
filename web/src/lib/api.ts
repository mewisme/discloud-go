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

/** Rebuild share links from the runtime API origin (API_URL inject). */
export function withPublicURLs(result: UploadResult): UploadResult {
  const id = result.fileId;
  const name = result.fileName;
  return {
    ...result,
    url: apiURL(`/f/${id}`),
    longURL: apiURL(`/f/${id}/${name}`),
    downloadURL: apiURL(`/f/${id}?download=1`),
    longDownloadURL: apiURL(`/f/${id}/${name}?download=1`),
  };
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

export interface FileInspect {
  fileId: string;
  fileName: string;
  fileSize: number;
  chunkSize: number;
  chunkCount: number;
  createdAt: string;
  views: number;
  downloads: number;
  ranges: number;
  bytesServed: number;
  uniqueVisitors: number;
  lastAccessAt?: string;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
}

export async function fetchFileInspect(
  fileId: string,
  init?: RequestInit,
): Promise<FileInspect> {
  const res = await fetch(apiURL(`/api/files/${fileId}/inspect`), {
    cache: "no-store",
    ...init,
  });
  if (res.status === 404) throw new Error("File not found on server");
  if (!res.ok) throw new Error(`Could not load inspect data (${res.status})`);
  return (await res.json()) as FileInspect;
}
