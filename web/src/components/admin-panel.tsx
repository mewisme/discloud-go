"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState, useSyncExternalStore } from "react";

import { PageBreadcrumb } from "@/components/page-breadcrumb";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Spinner } from "@/components/ui/spinner";
import {
  ApiError,
  fetchAdminOverview,
  type AdminOverview,
} from "@/lib/api";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  subscribeAuth,
} from "@/lib/auth";
import { formatBytes } from "@/lib/format";

function StatCard({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint?: string;
}) {
  return (
    <Card size="sm" className="gap-2">
      <CardHeader className="pb-0">
        <CardDescription>{label}</CardDescription>
        <CardTitle className="text-2xl tabular-nums tracking-tight">
          {value}
        </CardTitle>
      </CardHeader>
      {hint ? (
        <CardContent className="pt-0 text-xs text-muted-foreground">
          {hint}
        </CardContent>
      ) : null}
    </Card>
  );
}

function DepBadge({ ok, label }: { ok: boolean; label: string }) {
  return (
    <Badge variant={ok ? "secondary" : "destructive"}>
      {label}: {ok ? "ok" : "down"}
    </Badge>
  );
}

export function AdminPanel() {
  const router = useRouter();
  const user = useSyncExternalStore(
    subscribeAuth,
    getAuthSnapshot,
    getAuthServerSnapshot,
  );
  const [data, setData] = useState<AdminOverview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [fetchedAt, setFetchedAt] = useState<Date | null>(null);
  const [reloadToken, setReloadToken] = useState(0);

  useEffect(() => {
    void ensureAuth();
  }, []);

  useEffect(() => {
    if (user === null) {
      router.replace("/");
      return;
    }
    if (user && user.role !== "admin") {
      router.replace("/");
    }
  }, [user, router]);

  useEffect(() => {
    if (!user || user.role !== "admin") return;
    let cancelled = false;
    void fetchAdminOverview()
      .then((ov) => {
        if (cancelled) return;
        setData(ov);
        setFetchedAt(new Date());
        setError(null);
        setRefreshing(false);
      })
      .catch((err) => {
        if (cancelled) return;
        setError(
          err instanceof ApiError
            ? err.message
            : err instanceof Error
              ? err.message
              : "Failed to load overview",
        );
        setData(null);
        setRefreshing(false);
      });
    return () => {
      cancelled = true;
    };
  }, [user, reloadToken]);

  if (user === undefined || user === null || user.role !== "admin") {
    return (
      <div className="flex flex-col gap-6">
        <Skeleton className="h-8 w-40" />
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      </div>
    );
  }

  const loading = !data && !error;

  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Admin" }]} />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">Admin</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Instance overview. Traffic counters are lifetime totals.
          </p>
        </div>
        <div className="flex items-center gap-2">
          {fetchedAt ? (
            <span className="text-xs text-muted-foreground tabular-nums">
              Updated {fetchedAt.toLocaleTimeString()}
            </span>
          ) : null}
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={refreshing || loading}
            onClick={() => {
              setRefreshing(true);
              setReloadToken((n) => n + 1);
            }}
          >
            {refreshing ? <Spinner data-icon="inline-start" /> : null}
            Refresh
          </Button>
        </div>
      </div>

      {error ? (
        <Alert variant="destructive">
          <AlertTitle>Could not load overview</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {loading ? (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      ) : null}

      {data ? (
        <>
          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-medium">Storage</h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <StatCard label="Files" value={String(data.storage.fileCount)} />
              <StatCard
                label="Total bytes"
                value={formatBytes(data.storage.totalBytes)}
              />
              <StatCard
                label="Chunk store"
                value={String(data.storage.chunkStoreCount)}
              />
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-medium">Users & bots</h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <StatCard label="Users" value={String(data.users.count)} />
              <StatCard label="Admins" value={String(data.users.admins)} />
              <StatCard
                label="Bot tokens"
                value={String(data.bots.configured)}
              />
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-medium">Uploads (24h)</h2>
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              <StatCard
                label="Open sessions"
                value={String(data.uploads.openSessions)}
              />
              <StatCard
                label="Completed"
                value={String(data.uploads.completed24h)}
              />
              <StatCard
                label="Expired"
                value={String(data.uploads.expired24h)}
              />
              <StatCard
                label="Cancelled"
                value={String(data.uploads.cancelled24h)}
              />
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-medium">Traffic (lifetime)</h2>
            <div className="grid gap-3 sm:grid-cols-2">
              <StatCard
                label="Downloads"
                value={String(data.traffic.downloads)}
              />
              <StatCard
                label="Bytes served"
                value={formatBytes(data.traffic.bytesServed)}
              />
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h2 className="text-sm font-medium">Dependencies</h2>
            <div className="flex flex-wrap gap-2">
              <DepBadge ok={data.deps.postgres} label="Postgres" />
              <DepBadge ok={data.deps.valkey} label="Valkey" />
            </div>
          </section>
        </>
      ) : null}
    </div>
  );
}
