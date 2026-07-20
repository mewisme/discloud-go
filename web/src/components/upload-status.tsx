"use client";

import { useEffect, useSyncExternalStore } from "react";

import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@/components/ui/progress";
import {
  getState,
  isUploadOwner,
  subscribe,
} from "@/lib/upload-manager";

/** Global progress strip — stays mounted across route changes / tabs. */
export function UploadStatus() {
  const state = useSyncExternalStore(subscribe, getState, getState);
  // Local queue + owner-published pending (when mirroring another tab).
  const waiting = isUploadOwner()
    ? state.queue.length
    : state.queue.length + state.remoteQueue;

  useEffect(() => {
    // This tab owns the XHR, or still has File blobs waiting in its queue.
    if (!isUploadOwner() && state.queue.length === 0) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [state.uploading, state.queue.length]);

  if (!state.uploading) return null;

  return (
    <div className="border-b border-border/60 bg-muted/40 px-4 py-2">
      <div className="mx-auto w-full max-w-4xl">
        <Progress value={state.uploading.percent} className="gap-1">
          <ProgressLabel className="truncate text-xs">
            Uploading {state.uploading.fileName}
            {waiting > 0 ? ` · ${waiting} waiting` : ""}
          </ProgressLabel>
          <ProgressValue className="text-xs" />
        </Progress>
      </div>
    </div>
  );
}
