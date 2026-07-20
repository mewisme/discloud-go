"use client";

import { useSyncExternalStore } from "react";

function subscribe() {
  return () => { };
}

function useOrigin(): string {
  return useSyncExternalStore(
    subscribe,
    () => window.location.origin,
    () => "",
  );
}

/** Renders a code block with `$BASE` replaced by the current page origin. */
export function DocsCode({ children }: { children: string }) {
  const base = useOrigin();
  return (
    <pre className="overflow-x-auto rounded-lg border border-border/60 bg-muted/50 p-4 text-xs leading-relaxed">
      <code className="font-mono">
        {children.replaceAll("$BASE", base || "…")}
      </code>
    </pre>
  );
}

/** Current site origin for the API docs intro. */
export function DocsOrigin() {
  const base = useOrigin();
  return (
    <code className="font-mono text-foreground">{base || "…"}</code>
  );
}
