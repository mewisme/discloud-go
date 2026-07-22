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

/** Code block with `$BASE` → API origin and `$WEB` → this page's origin. */
export function DocsCode({ children }: { children: string }) {
  const base = useApiBase();
  const web = useSyncExternalStore(
    subscribe,
    () => (typeof window !== "undefined" ? window.location.origin : ""),
    () => "",
  );
  const text = children
    .replaceAll("$BASE", base)
    .replaceAll("$WEB", web || "http://localhost:3000");
  return (
    <pre className="overflow-x-auto rounded-lg border border-border/60 bg-muted/40 p-3.5 text-[13px] leading-relaxed">
      <code className="font-mono whitespace-pre">{text}</code>
    </pre>
  );
}

export function DocsOrigin() {
  const base = useApiBase();
  return <code className="font-mono text-foreground">{base}</code>;
}
