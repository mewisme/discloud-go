import type { UploadResult } from "@/lib/api";
import { stripTokenFromURL } from "@/lib/api";

const STORAGE_KEY = "discloud:files";
const CHANGE_EVENT = "discloud:files";

export interface LocalFile {
  fileId: string;
  fileName: string;
  fileSize: number;
  createdAt: string;
  longURL: string;
  longDownloadURL: string;
}

const EMPTY: LocalFile[] = [];
const listeners = new Set<() => void>();

let snapshot: LocalFile[] = EMPTY;
let hydrated = false;

function readStorage(): LocalFile[] {
  if (typeof window === "undefined") return EMPTY;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return EMPTY;
    const parsed = JSON.parse(raw) as LocalFile[];
    if (!Array.isArray(parsed)) return EMPTY;
    return parsed.map((f) => ({
      ...f,
      longURL: stripTokenFromURL(f.longURL ?? ""),
      longDownloadURL: stripTokenFromURL(f.longDownloadURL ?? ""),
    }));
  } catch {
    return EMPTY;
  }
}

function ensureHydrated(): void {
  if (!hydrated && typeof window !== "undefined") {
    snapshot = readStorage();
    hydrated = true;
  }
}

function emit(next: LocalFile[]): void {
  snapshot = next;
  hydrated = true;
  for (const listener of listeners) listener();
}

/** Stable snapshot for useSyncExternalStore — same ref until data changes. */
export function getLocalFilesSnapshot(): LocalFile[] {
  ensureHydrated();
  return snapshot;
}

export function getLocalFilesServerSnapshot(): LocalFile[] {
  return EMPTY;
}

export function subscribeLocalFiles(onChange: () => void): () => void {
  ensureHydrated();
  listeners.add(onChange);

  const onStorage = (e: StorageEvent) => {
    if (e.key !== STORAGE_KEY) return;
    snapshot = readStorage();
    onChange();
  };
  const onCustom = () => {
    snapshot = readStorage();
    onChange();
  };

  if (typeof window !== "undefined") {
    window.addEventListener("storage", onStorage);
    window.addEventListener(CHANGE_EVENT, onCustom);
  }

  return () => {
    listeners.delete(onChange);
    if (typeof window !== "undefined") {
      window.removeEventListener("storage", onStorage);
      window.removeEventListener(CHANGE_EVENT, onCustom);
    }
  };
}

export function listLocalFiles(): LocalFile[] {
  ensureHydrated();
  return snapshot;
}

/** Persist upload metadata locally (newest first). Dedupes by fileId. */
export function rememberLocalFile(result: UploadResult): LocalFile {
  ensureHydrated();
  const entry: LocalFile = {
    fileId: result.fileId,
    fileName: result.fileName,
    fileSize: result.fileSize,
    createdAt: new Date().toISOString(),
    longURL: stripTokenFromURL(result.longURL),
    longDownloadURL: stripTokenFromURL(result.longDownloadURL),
  };
  const next = [entry, ...snapshot.filter((f) => f.fileId !== entry.fileId)];
  localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  emit(next);
  return entry;
}

export function removeLocalFile(fileId: string): void {
  ensureHydrated();
  const next = snapshot.filter((f) => f.fileId !== fileId);
  localStorage.setItem(STORAGE_KEY, JSON.stringify(next));
  emit(next);
}
