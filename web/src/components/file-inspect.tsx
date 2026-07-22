"use client";

import { Download, ExternalLink, RefreshCw, Share2 } from "lucide-react";
import Link from "next/link";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { CopyButton } from "@/components/copy-button";
import { ShareQR } from "@/components/share-qr";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { fetchFileInspect, type FileInspect } from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";

function mimeKind(name: string): "image" | "video" | "other" {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "avif"].includes(ext)) {
    return "image";
  }
  if (["mp4", "webm", "ogg", "mov"].includes(ext)) return "video";
  return "other";
}

export function FileInspectPanel({ fileId }: { fileId: string }) {
  const [data, setData] = useState<FileInspect | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [showPreview, setShowPreview] = useState(false);

  useEffect(() => {
    const ac = new AbortController();
    fetchFileInspect(fileId, { signal: ac.signal })
      .then((info) => {
        if (ac.signal.aborted) return;
        setData(info);
        setError(null);
      })
      .catch((err: unknown) => {
        if (ac.signal.aborted) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : "Failed to load");
      })
      .finally(() => {
        if (!ac.signal.aborted) setLoading(false);
      });
    return () => ac.abort();
  }, [fileId]);

  const refresh = () => {
    setLoading(true);
    setError(null);
    fetchFileInspect(fileId)
      .then((info) => {
        setData(info);
        setError(null);
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : "Failed to load");
      })
      .finally(() => setLoading(false));
  };

  if (loading && !data) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (error && !data) {
    return (
      <p className="text-sm text-destructive" role="alert">
        {error}
      </p>
    );
  }

  if (!data) return null;

  const kind = mimeKind(data.fileName);
  const inspectPath = `/i/${data.fileId}`;

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="truncate text-2xl font-semibold tracking-tight">
            {data.fileName}
          </h1>
          <p className="mt-1 font-mono text-xs text-muted-foreground">
            {data.fileId}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={async () => {
              try {
                if (typeof navigator.share === "function") {
                  await navigator.share({
                    title: data.fileName,
                    url: data.longURL,
                  });
                  return;
                }
                await navigator.clipboard.writeText(data.longURL);
                toast.success("Link copied to clipboard");
              } catch (err) {
                if (err instanceof DOMException && err.name === "AbortError") {
                  return;
                }
                toast.error("Could not share link");
              }
            }}
          >
            <Share2 aria-hidden /> Share
          </Button>
          <a
            href={data.longURL}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            <ExternalLink aria-hidden /> Open
          </a>
          <a
            href={data.longDownloadURL}
            className={buttonVariants({ variant: "default", size: "sm" })}
          >
            <Download aria-hidden /> Download
          </a>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={refresh}
          >
            <RefreshCw aria-hidden /> Refresh
          </Button>
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <Stat label="Views" value={String(data.views)} />
        <Stat label="Downloads" value={String(data.downloads)} />
        <Stat label="Ranges" value={String(data.ranges)} />
        <Stat label="Uniques" value={String(data.uniqueVisitors)} />
        <Stat label="Bytes served" value={formatBytes(data.bytesServed)} />
        <Stat label="Size" value={formatBytes(data.fileSize)} />
      </div>

      <p className="text-xs text-muted-foreground">
        {data.chunkCount} chunk{data.chunkCount === 1 ? "" : "s"} ×{" "}
        {formatBytes(data.chunkSize)} · Uploaded {formatDate(data.createdAt)}
        {data.lastAccessAt
          ? ` · Last access ${formatDate(data.lastAccessAt)}`
          : ""}
      </p>

      {(kind === "image" || kind === "video") && (
        <Card>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-2">
              <h2 className="text-sm font-semibold">Preview</h2>
              {!showPreview && (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => setShowPreview(true)}
                >
                  Load preview
                </Button>
              )}
            </div>
            {showPreview ? (
              kind === "image" ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={data.url}
                  alt={data.fileName}
                  className="max-h-96 w-full rounded-lg object-contain bg-muted"
                />
              ) : (
                <video
                  src={data.url}
                  controls
                  className="max-h-96 w-full rounded-lg bg-muted"
                />
              )
            ) : (
              <p className="text-sm text-muted-foreground">
                Not loaded — avoids pulling the file until you ask.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-[160px_1fr]">
        <ShareQR value={data.longURL} />
        <div className="flex flex-col gap-3">
          <LinkRow label="Share link" href={data.longURL} />
          <LinkRow label="Download" href={data.longDownloadURL} />
          <LinkRow
            label="Inspect"
            href={
              typeof window !== "undefined"
                ? `${window.location.origin}${inspectPath}`
                : inspectPath
            }
          />
        </div>
      </div>

      {error && (
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      <p className="text-xs text-muted-foreground">
        <Link href="/files" className="underline-offset-2 hover:underline">
          ← Back to files
        </Link>
      </p>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card>
      <CardContent className="gap-1 py-4">
        <p className="text-xs font-medium text-muted-foreground">{label}</p>
        <p className="text-lg font-semibold tabular-nums tracking-tight">
          {value}
        </p>
      </CardContent>
    </Card>
  );
}

function LinkRow({ label, href }: { label: string; href: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex items-center gap-2">
        <Input
          readOnly
          value={href}
          className="font-mono text-xs"
          aria-label={label}
        />
        <CopyButton value={href} label={`Copy ${label.toLowerCase()}`} />
      </div>
    </div>
  );
}
