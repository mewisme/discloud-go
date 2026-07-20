export interface StoredFile {
  fileId: string;
  fileName: string;
  fileSize: number;
  chunkSize: number;
  createdAt: string;
}

export interface UploadResult {
  fileId: string;
  fileName: string;
  fileSize: number;
  url: string;
  longURL: string;
  downloadURL: string;
  longDownloadURL: string;
}

/** API origin for server-side fetches (inside Docker: http://api:8080). */
export const API_URL = process.env.API_URL ?? "http://localhost:8080";

export async function getFiles(): Promise<StoredFile[]> {
  const res = await fetch(`${API_URL}/api/files`, { cache: "no-store" });
  if (!res.ok) throw new Error(`API responded ${res.status}`);
  const data = (await res.json()) as { files: StoredFile[] };
  return data.files;
}
