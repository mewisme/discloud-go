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

/** API origin for server-side fetches (inside Docker: http://api:8080). */
export const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function fetchFileMeta(
  fileId: string,
  init?: RequestInit,
): Promise<FileMeta> {
  const res = await fetch(`/api/files/${fileId}`, {
    cache: "no-store",
    ...init,
  });
  if (res.status === 404) throw new Error("File not found on server");
  if (!res.ok) throw new Error(`Could not load metadata (${res.status})`);
  return (await res.json()) as FileMeta;
}
