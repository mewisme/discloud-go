import { toast } from "sonner";

import type { UploadResult } from "@/lib/api";
import { uploadFileChunked } from "@/lib/chunked-upload";
import { rememberLocalFile } from "@/lib/local-files";

export type QueuedItem = { id: string; name: string; size: number };

export type UploadManagerState = {
  /** Active upload (this tab or mirrored from another). */
  uploading: { fileName: string; percent: number } | null;
  /** Files waiting in this tab (File blobs are tab-local). */
  queue: QueuedItem[];
  /** Extra pending count published by the owner tab (for peer UIs). */
  remoteQueue: number;
  lastResult: UploadResult | null;
};

type Job = { id: string; file: File };

type Listener = (state: UploadManagerState) => void;

const STORAGE_KEY = "discloud:upload";
const CHANNEL_NAME = "discloud-upload";
const STALE_MS = 15_000;

type WireState = {
  ownerId: string | null;
  updatedAt: number;
  uploading: { fileName: string; percent: number } | null;
  queue: number;
  lastResult: UploadResult | null;
};

let uploading: UploadManagerState["uploading"] = null;
let lastResult: UploadResult | null = null;
let remoteQueue = 0;
let jobs: Job[] = [];
let running = false;
let ownerId: string | null = null;
let tabId = "";
let channel: BroadcastChannel | null = null;
let synced = false;
let lastPublishedPercent = -1;
let heartbeat: ReturnType<typeof setInterval> | null = null;
let cached: UploadManagerState = {
  uploading: null,
  queue: [],
  remoteQueue: 0,
  lastResult: null,
};
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
    name: j.file.name,
    size: j.file.size,
  }));
}

function snapshot(): UploadManagerState {
  cached = {
    uploading,
    queue: queueView(),
    remoteQueue: ownerId === ensureTabId() ? 0 : remoteQueue,
    lastResult,
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

function wirePayload(): WireState {
  return {
    ownerId,
    updatedAt: now(),
    uploading,
    queue: jobs.length,
    lastResult,
  };
}

function writeStored(wire: WireState): void {
  if (typeof localStorage === "undefined") return;
  try {
    if (!wire.uploading && wire.queue === 0 && !wire.lastResult) {
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

  const wasDone = lastResult?.fileId;
  uploading = wire.uploading;
  ownerId = wire.ownerId;
  remoteQueue = wire.queue ?? 0;
  if (wire.lastResult) lastResult = wire.lastResult;
  notify();

  if (wire.lastResult && wire.lastResult.fileId !== wasDone) {
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
    uploading = stored.uploading;
    ownerId = stored.ownerId;
    remoteQueue = stored.queue ?? 0;
    if (stored.lastResult) lastResult = stored.lastResult;
  }
  snapshot();

  try {
    channel = new BroadcastChannel(CHANNEL_NAME);
    channel.onmessage = (e: MessageEvent<WireState>) => applyRemote(e.data);
  } catch {
    channel = null;
  }
  window.addEventListener("storage", onStorage);

  window.addEventListener("pagehide", () => {
    if (ownerId === ensureTabId() && (uploading || jobs.length > 0)) {
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
        lastResult,
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
  const id = ensureTabId();
  ownerId = id;
  uploading = { fileName: next.file.name, percent: 0 };
  publish();

  try {
    const result = await uploadFileChunked(next.file, (sent, total) => {
      const percent = Math.round((sent / total) * 100);
      if (
        percent === lastPublishedPercent &&
        uploading?.fileName === next.file.name
      ) {
        return;
      }
      uploading = { fileName: next.file.name, percent };
      publish();
    });
    rememberLocalFile(result);
    window.dispatchEvent(new Event("discloud:files"));
    lastResult = result;
    toast.success(`${result.fileName} uploaded`);
  } catch (err: unknown) {
    toast.error(err instanceof Error ? err.message : "Upload failed", {
      description:
        "Retrying the upload will skip chunks that already made it through.",
    });
  } finally {
    uploading = null;
    running = false;
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

export function isUploadOwner(): boolean {
  return running && ownerId === ensureTabId();
}

/** Enqueue one or more files. Safe to call while uploads are running. */
export function enqueue(files: Iterable<File>): void {
  syncFromPeers();
  const list = [...files].filter((f) => f.size > 0);
  if (list.length === 0) {
    toast.error("No non-empty files selected");
    return;
  }

  for (const file of list) {
    jobs.push({
      id:
        typeof crypto !== "undefined" && "randomUUID" in crypto
          ? crypto.randomUUID()
          : `${file.name}-${file.size}-${Math.random()}`,
      file,
    });
  }

  toast.message(
    list.length === 1
      ? `Queued ${list[0].name}`
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

export function clearDone(): void {
  syncFromPeers();
  if (lastResult && !uploading && jobs.length === 0) {
    lastResult = null;
    publish();
  }
}
