"use client";

import { apiURL } from "@/lib/api";
import { TriangleAlert } from "lucide-react";
import { useEffect, useState } from "react";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

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
    <Alert
      className="rounded-none border-x-0 border-t-0 border-geist-amber/30 bg-geist-amber-soft text-geist-amber *:data-[slot=alert-description]:text-geist-amber"
    >
      <TriangleAlert aria-hidden />
      <AlertTitle>Service degraded</AlertTitle>
      <AlertDescription>
        The storage backend is currently unreachable. Uploads and downloads may
        fail.
      </AlertDescription>
    </Alert>
  );
}
