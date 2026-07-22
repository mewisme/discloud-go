"use client";

import { apiURL } from "@/lib/api";
import { TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

export function HealthBanner() {
  const [degraded, setDegraded] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    fetch(apiURL("/readyz"), { signal: controller.signal, cache: "no-store" })
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
      className="flex items-center gap-2 border-b border-geist-amber/30 bg-geist-amber-soft px-4 py-2 text-sm text-geist-amber"
    >
      <TriangleAlert className="size-4 shrink-0" aria-hidden />
      Service degraded: the storage backend is currently unreachable. Uploads and downloads may fail.
    </div>
  );
}
