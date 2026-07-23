/** IndexedDB persistence for resumable upload sessions across page reloads. */

const DB_NAME = "discloud-uploads";
const STORE = "sessions";
const DB_VERSION = 1;

export type UploadRecord = {
  fingerprint: string;
  uploadId: string;
  resumeToken: string;
  fileName: string;
  fileSize: number;
  lastModified: number;
  chunkCount: number;
};

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION);
    req.onerror = () => reject(req.error ?? new Error("indexedDB open failed"));
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) {
        db.createObjectStore(STORE, { keyPath: "fingerprint" });
      }
    };
    req.onsuccess = () => resolve(req.result);
  });
}

export async function saveUploadRecord(rec: UploadRecord): Promise<void> {
  try {
    const db = await openDB();
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).put(rec);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
    db.close();
  } catch {
    /* ignore persistence failures */
  }
}

export async function findUploadRecord(
  fingerprint: string,
): Promise<UploadRecord | null> {
  try {
    const db = await openDB();
    const rec = await new Promise<UploadRecord | null>((resolve, reject) => {
      const tx = db.transaction(STORE, "readonly");
      const req = tx.objectStore(STORE).get(fingerprint);
      req.onsuccess = () => resolve((req.result as UploadRecord) ?? null);
      req.onerror = () => reject(req.error);
    });
    db.close();
    return rec;
  } catch {
    return null;
  }
}

export async function clearUploadRecord(fingerprint: string): Promise<void> {
  try {
    const db = await openDB();
    await new Promise<void>((resolve, reject) => {
      const tx = db.transaction(STORE, "readwrite");
      tx.objectStore(STORE).delete(fingerprint);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
    db.close();
  } catch {
    /* ignore */
  }
}

export async function listUploadRecords(): Promise<UploadRecord[]> {
  try {
    const db = await openDB();
    const rows = await new Promise<UploadRecord[]>((resolve, reject) => {
      const tx = db.transaction(STORE, "readonly");
      const req = tx.objectStore(STORE).getAll();
      req.onsuccess = () => resolve((req.result as UploadRecord[]) ?? []);
      req.onerror = () => reject(req.error);
    });
    db.close();
    return rows;
  } catch {
    return [];
  }
}
