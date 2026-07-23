import { toast } from "sonner";

import { stripTokenFromURL, withPublicURLs, type UploadResult } from "@/lib/api";
import { isAbortError, cancelUploadSession, uploadFileChunked } from "@/lib/chunked-upload";
import { rememberLocalFile } from "@/lib/local-files";
import type { NamedFile } from "@/lib/folder-files";

export type QueuedItem = { id: string; name: string; size: number };

export type UploadPhase = "uploading" | "processing";

export type ActiveUpload = {
  fileName: string;
  percent: number;
  phase: UploadPhase;
  sent: number;
  total: number;
  /** Instantaneous throughput (bytes/sec); 0 when unknown. */
  bytesPerSec: number;
};

export type UploadManagerState = {
  /** Active upload (this tab or mirrored from another). */
  uploading: ActiveUpload | null;
  /** True when this tab owns the in-flight upload (can cancel). */
  canCancel: boolean;
  /** Files waiting in this tab (File blobs are tab-local). */
  queue: QueuedItem[];
  /** Extra pending count published by the owner tab (for peer UIs). */
  remoteQueue: number;
  /** Completed uploads this session (newest first). */
  results: UploadResult[];
  /** Failed jobs that can be retried (same File still held). */
  failed: QueuedItem[];
};

type Job = { id: string; file: File; relativePath?: string };

type Listener = (state: UploadManagerState) => void;

const STORAGE_KEY = "discloud:upload";
const CHANNEL_NAME = "discloud-upload";
const STALE_MS = 15_000;

type WireState = {
  ownerId: string | null;
  updatedAt: number;
  uploading: ActiveUpload | null;
  queue: number;
  results: UploadResult[];
  /** @deprecated old single-result wire; still accepted when reading storage. */
  lastResult?: UploadResult | null;
};

function normalizeActive(
  u: ActiveUpload | { fileName: string; percent: number } | null,
): ActiveUpload | null {
  if (!u) return null;
  const full = u as Partial<ActiveUpload>;
  return {
    fileName: u.fileName,
    percent: u.percent,
    phase: full.phase === "processing" ? "processing" : "uploading",
    sent: typeof full.sent === "number" ? full.sent : 0,
    total: typeof full.total === "number" ? full.total : 0,
    bytesPerSec: typeof full.bytesPerSec === "number" ? full.bytesPerSec : 0,
  };
}

let uploading: UploadManagerState["uploading"] = null;
let results: UploadResult[] = [];
let remoteQueue = 0;
let jobs: Job[] = [];
let failedJobs: Job[] = [];
let running = false;
let abortCtrl: AbortController | null = null;
let ownerId: string | null = null;
let tabId = "";
let channel: BroadcastChannel | null = null;
let synced = false;
let lastPublishedPercent = -1;
let heartbeat: ReturnType<typeof setInterval> | null = null;
let cached: UploadManagerState = {
  uploading: null,
  canCancel: false,
  queue: [],
  remoteQueue: 0,
  results: [],
  failed: [],
};

/** Newest-first upsert; keeps peer/local results without dropping siblings. */
function upsertResults(incoming: UploadResult[]): void {
  if (incoming.length === 0) return;
  const ids = new Set(incoming.map((r) => r.fileId));
  results = [...incoming, ...results.filter((r) => !ids.has(r.fileId))];
}

function wireResults(wire: WireState): UploadResult[] {
  const raw =
    Array.isArray(wire.results) && wire.results.length > 0
      ? wire.results
      : wire.lastResult
        ? [wire.lastResult]
        : [];
  return raw.map(redactResult);
}

const listeners = new Set<Listener>();

function now(): number {
  return Date.now();
}

function ensureTabId(): string {
  if (!tabId && typeof crypto !== "undefined" && "randomUUID" in crypto) {
    tabId = crypto.randomUUID();
  }
  return tabId || "tab";
}

function queueView(): QueuedItem[] {
  return jobs.map((j) => ({
    id: j.id,
    name: j.relativePath || j.file.name,
    size: j.file.size,
  }));
}

function snapshot(): UploadManagerState {
  cached = {
    uploading,
    canCancel: running,
    queue: queueView(),
    remoteQueue: ownerId === ensureTabId() ? 0 : remoteQueue,
    results,
    failed: failedJobs.map((j) => ({
      id: j.id,
      name: j.relativePath || j.file.name,
      size: j.file.size,
    })),
  };
  return cached;
}

function stopHeartbeat(): void {
  if (heartbeat) {
    clearInterval(heartbeat);
    heartbeat = null;
  }
}

function startHeartbeat(): void {
  stopHeartbeat();
  heartbeat = setInterval(() => {
    if (ownerId !== ensureTabId() || !uploading) {
      stopHeartbeat();
      return;
    }
    writeAndBroadcast(wirePayload());
  }, 5_000);
}

function redactResult(r: UploadResult): UploadResult {
  return {
    fileId: r.fileId,
    fileName: r.fileName,
    fileSize: r.fileSize,
    visibility: r.visibility,
    status: r.status,
    ownedByCurrentUser: r.ownedByCurrentUser,
    createdAt: r.createdAt,
    expiresAt: r.expiresAt,
    url: stripTokenFromURL(r.url),
    longURL: stripTokenFromURL(r.longURL),
    downloadURL: stripTokenFromURL(r.downloadURL),
    longDownloadURL: stripTokenFromURL(r.longDownloadURL),
  };
}

function wirePayload(): WireState {
  return {
    ownerId,
    updatedAt: now(),
    uploading,
    queue: jobs.length,
    results: results.map(redactResult),
  };
}

function writeStored(wire: WireState): void {
  if (typeof localStorage === "undefined") return;
  try {
    if (!wire.uploading && wire.queue === 0 && wire.results.length === 0) {
      localStorage.removeItem(STORAGE_KEY);
    } else {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(wire));
    }
  } catch {
    // best-effort
  }
}

function writeAndBroadcast(wire: WireState): void {
  writeStored(wire);
  channel?.postMessage(wire);
}

function notify(): void {
  const snap = snapshot();
  for (const listener of listeners) listener(snap);
}


function readStored(): WireState | null {
  if (typeof localStorage === "undefined") return null;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as WireState;
    if (
      parsed.uploading &&
      now() - (parsed.updatedAt ?? 0) > STALE_MS
    ) {
      localStorage.removeItem(STORAGE_KEY);
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function peerBusy(): boolean {
  const peers = readStored();
  if (!peers?.uploading) return false;
  if (peers.ownerId === ensureTabId()) return false;
  return now() - (peers.updatedAt ?? 0) <= STALE_MS;
}

/** Publish from the owner tab (or clearDone). */
function publish(): void {
  const id = ensureTabId();
  if (uploading) {
    ownerId = id;
    lastPublishedPercent = uploading.percent;
    startHeartbeat();
  } else {
    if (jobs.length === 0) ownerId = null;
    lastPublishedPercent = -1;
    stopHeartbeat();
  }
  notify();
  writeAndBroadcast(wirePayload());
}

function applyRemote(wire: WireState): void {
  if (
    wire.uploading &&
    now() - (wire.updatedAt ?? 0) > STALE_MS
  ) {
    return;
  }
  // Owner tab ignores its own broadcasts.
  if (wire.ownerId === ensureTabId() && running) return;

  const known = new Set(results.map((r) => r.fileId));
  uploading = normalizeActive(wire.uploading);
  ownerId = wire.ownerId;
  remoteQueue = wire.queue ?? 0;
  upsertResults(wireResults(wire));
  notify();

  if (results.some((r) => !known.has(r.fileId))) {
    window.dispatchEvent(new Event("discloud:files"));
  }

  // Peer finished → start our local queue if any.
  if (!wire.uploading && jobs.length > 0 && !running) {
    void pump();
  }
}

function onStorage(e: StorageEvent): void {
  if (e.key !== STORAGE_KEY) return;
  if (!e.newValue) {
    if (!running) {
      uploading = null;
      remoteQueue = 0;
      ownerId = null;
      notify();
    }
    return;
  }
  try {
    applyRemote(JSON.parse(e.newValue) as WireState);
  } catch {
    // ignore
  }
}

function syncFromPeers(): void {
  if (synced || typeof window === "undefined") return;
  synced = true;
  ensureTabId();

  const stored = readStored();
  if (stored) {
    uploading = normalizeActive(stored.uploading);
    ownerId = stored.ownerId;
    remoteQueue = stored.queue ?? 0;
    upsertResults(wireResults(stored));
  }
  snapshot();

  try {
    channel = new BroadcastChannel(CHANNEL_NAME);
    channel.onmessage = (e: MessageEvent<WireState>) => applyRemote(e.data);
  } catch {
    channel = null;
  }
  window.addEventListener("storage", onStorage);

  window.addEventListener("beforeunload", (e) => {
    if (running || jobs.length > 0) {
      e.preventDefault();
      e.returnValue = "";
    }
  });

  window.addEventListener("pagehide", () => {
    if (ownerId === ensureTabId() && (uploading || jobs.length > 0)) {
      abortCtrl?.abort();
      abortCtrl = null;
      jobs = [];
      uploading = null;
      running = false;
      ownerId = null;
      stopHeartbeat();
      writeAndBroadcast({
        ownerId: null,
        updatedAt: now(),
        uploading: null,
        queue: 0,
        results,
      });
      notify();
    }
  });
}

async function pump(): Promise<void> {
  if (running) return;
  if (peerBusy()) {
    notify();
    return;
  }

  const next = jobs.shift();
  if (!next) {
    publish();
    return;
  }

  running = true;
  abortCtrl = new AbortController();
  const sessionBox: {
    current: { uploadId: string; resumeToken: string } | null;
  } = { current: null };
  const id = ensureTabId();
  ownerId = id;
  const displayName = next.relativePath || next.file.name;
  uploading = {
    fileName: displayName,
    percent: 0,
    phase: "uploading",
    sent: 0,
    total: next.file.size,
    bytesPerSec: 0,
  };
  publish();

  let speedSent = 0;
  let speedAt = now();
  let bytesPerSec = 0;
  let lastUIPublish = 0;

  try {
    const result = await uploadFileChunked(next.file, {
      relativePath: next.relativePath,
      signal: abortCtrl.signal,
      onSession: (uploadId, resumeToken) => {
        sessionBox.current = { uploadId, resumeToken };
      },
      onProgress: (sent, total) => {
        const t = now();
        if (t - speedAt >= 300) {
          const dt = (t - speedAt) / 1000;
          bytesPerSec = dt > 0 ? (sent - speedSent) / dt : 0;
          speedSent = sent;
          speedAt = t;
        }
        const percent = total > 0 ? Math.round((sent / total) * 100) : 0;
        const samePct =
          percent === lastPublishedPercent &&
          uploading?.fileName === displayName &&
          uploading.phase === "uploading";
        if (samePct && t - lastUIPublish < 200) return;
        lastUIPublish = t;
        uploading = {
          fileName: displayName,
          percent,
          phase: "uploading",
          sent,
          total,
          bytesPerSec,
        };
        publish();
      },
      onProcessing: () => {
        uploading = {
          fileName: displayName,
          percent: 100,
          phase: "processing",
          sent: next.file.size,
          total: next.file.size,
          bytesPerSec: 0,
        };
        publish();
      },
    });
    const publicResult = withPublicURLs(result);
    rememberLocalFile(publicResult);
    window.dispatchEvent(new Event("discloud:files"));
    upsertResults([publicResult]);
    toast.success(`${publicResult.fileName} uploaded`);
  } catch (err: unknown) {
    if (isAbortError(err)) {
      const s = sessionBox.current;
      if (s) {
        void cancelUploadSession(s.uploadId, s.resumeToken);
      }
      toast.message(`Cancelled ${displayName}`);
    } else {
      failedJobs = [next, ...failedJobs.filter((j) => j.id !== next.id)];
      toast.error(err instanceof Error ? err.message : "Upload failed", {
        description: "Use Retry on the failed item to resume.",
      });
    }
  } finally {
    uploading = null;
    running = false;
    abortCtrl = null;
    publish();
    if (jobs.length > 0) void pump();
  }
}

/** Stable getSnapshot for useSyncExternalStore (same ref until notify). */
export function getState(): UploadManagerState {
  syncFromPeers();
  return cached;
}

export function subscribe(listener: Listener): () => void {
  syncFromPeers();
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Enqueue one or more files. Safe to call while uploads are running. */
export function enqueue(files: Iterable<File> | Iterable<NamedFile>): void {
  syncFromPeers();
  const named: NamedFile[] = [];
  for (const item of files) {
    if (item instanceof File) {
      named.push({
        file: item,
        relativePath: item.webkitRelativePath || undefined,
      });
    } else if (item && typeof item === "object" && "file" in item) {
      named.push(item as NamedFile);
    }
  }
  const list = named.filter((n) => n.file.size > 0);
  if (list.length === 0) {
    toast.error("No non-empty files selected");
    return;
  }

  for (const { file, relativePath } of list) {
    jobs.push({
      id:
        typeof crypto !== "undefined" && "randomUUID" in crypto
          ? crypto.randomUUID()
          : `${file.name}-${file.size}-${Math.random()}`,
      file,
      relativePath,
    });
  }

  toast.message(
    list.length === 1
      ? `Queued ${list[0].relativePath || list[0].file.name}`
      : `Queued ${list.length} files`,
  );
  publish();
  void pump();
}

/** @deprecated use enqueue — kept as a thin alias for a single file. */
export function start(file: File): void {
  enqueue([file]);
}

export function removeQueued(id: string): void {
  syncFromPeers();
  const before = jobs.length;
  jobs = jobs.filter((j) => j.id !== id);
  if (jobs.length !== before) publish();
}

/** Abort the in-flight upload owned by this tab (queue is kept). */
export function cancelUpload(): void {
  syncFromPeers();
  abortCtrl?.abort();
}

/** Re-queue a failed upload for resume. */
export function retryFailed(id: string): void {
  syncFromPeers();
  const job = failedJobs.find((j) => j.id === id);
  if (!job) return;
  failedJobs = failedJobs.filter((j) => j.id !== id);
  jobs.push(job);
  publish();
  void pump();
}

export function dismissFailed(id: string): void {
  syncFromPeers();
  const before = failedJobs.length;
  failedJobs = failedJobs.filter((j) => j.id !== id);
  if (failedJobs.length !== before) publish();
}

export function dismissResult(fileId: string): void {
  syncFromPeers();
  const next = results.filter((r) => r.fileId !== fileId);
  if (next.length !== results.length) {
    results = next;
    publish();
  }
}

export function dismissAllResults(): void {
  syncFromPeers();
  if (results.length === 0) return;
  results = [];
  publish();
}

export function clearDone(): void {
  syncFromPeers();
  if (results.length > 0 && !uploading && jobs.length === 0) {
    results = [];
    publish();
  }
}
