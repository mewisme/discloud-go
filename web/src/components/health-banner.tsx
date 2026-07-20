"use client";

import { TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

export function HealthBanner() {
  const [degraded, setDegraded] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    fetch("/readyz", { signal: controller.signal, cache: "no-store" })
      .then((res) => setDegraded(!res.ok))
      .catch((err: unknown) => {
        if (!(err instanceof DOMException && err.name === "AbortError")) {
          setDegraded(true);
        }
      });
    return () => controller.abort();
  }, []);

  if (!degraded) return null;
  return (
    <div
      role="status"
      className="flex items-center gap-2 border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-sm text-amber-700 dark:text-amber-400"
    >
      <TriangleAlert className="size-4 shrink-0" aria-hidden />
      Service degraded: the storage backend is currently unreachable. Uploads and downloads may fail.
    </div>
  );
}
