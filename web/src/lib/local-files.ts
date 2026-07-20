import type { UploadResult } from "@/lib/api";

const STORAGE_KEY = "discloud:files";

export interface LocalFile {
  fileId: string;
  fileName: string;
  fileSize: number;
  createdAt: string;
  longURL: string;
  longDownloadURL: string;
}

export function listLocalFiles(): LocalFile[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as LocalFile[];
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/** Persist upload metadata locally (newest first). Dedupes by fileId. */
export function rememberLocalFile(result: UploadResult): LocalFile {
  const entry: LocalFile = {
    fileId: result.fileId,
    fileName: result.fileName,
    fileSize: result.fileSize,
    createdAt: new Date().toISOString(),
    longURL: result.longURL,
    longDownloadURL: result.longDownloadURL,
  };
  const next = [entry, ...listLocalFiles().filter((f) => f.fileId !== entry.fileId)];
  localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  return entry;
}

export function removeLocalFile(fileId: string): void {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify(listLocalFiles().filter((f) => f.fileId !== fileId)),
  );
}
