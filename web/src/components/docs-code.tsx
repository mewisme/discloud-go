"use client";

import { useSyncExternalStore } from "react";

import { apiURL } from "@/lib/api";

function subscribe() {
  return () => { };
}

function useApiBase(): string {
  return useSyncExternalStore(
    subscribe,
    () => apiURL("").replace(/\/$/, "") || "http://localhost:8080",
    () => "http://localhost:8080",
  );
}

/** Code block with `$BASE` replaced by the runtime API origin. */
export function DocsCode({ children }: { children: string }) {
  const base = useApiBase();
  return (
    <pre className="overflow-x-auto rounded-lg border border-border/60 bg-muted/40 p-3.5 text-[13px] leading-relaxed">
      <code className="font-mono whitespace-pre">{children.replaceAll("$BASE", base)}</code>
    </pre>
  );
}

export function DocsOrigin() {
  const base = useApiBase();
  return <code className="font-mono text-foreground">{base}</code>;
}
