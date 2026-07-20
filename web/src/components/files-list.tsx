"use client";

import { Download, ExternalLink, FolderOpen, Trash2, X } from "lucide-react";
import { useEffect, useState, useSyncExternalStore } from "react";

import { CopyButton } from "@/components/copy-button";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { fetchFileMeta, apiURL, type FileMeta } from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";
import {
  getLocalFilesServerSnapshot,
  getLocalFilesSnapshot,
  removeLocalFile,
  subscribeLocalFiles,
  type LocalFile,
} from "@/lib/local-files";
import { cn } from "@/lib/utils";

export function FilesList() {
  const files = useSyncExternalStore(
    subscribeLocalFiles,
    getLocalFilesSnapshot,
    getLocalFilesServerSnapshot,
  );
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const selected = files.find((f) => f.fileId === selectedId) ?? null;

  if (files.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
        <FolderOpen className="size-6 text-muted-foreground" aria-hidden />
        <h1 className="font-semibold">No files yet</h1>
        <p className="max-w-sm text-sm text-muted-foreground">
          Files you upload in this browser are saved here. Upload something on
          the home page to get started.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Your files</h1>
        <Badge variant="secondary">
          {files.length} file{files.length === 1 ? "" : "s"}
        </Badge>
      </div>
      <p className="text-sm text-muted-foreground">
        Stored in this browser only — clearing site data removes the list, not
        the files on the server. Click a name for metadata.
      </p>
      <div className="overflow-hidden rounded-xl border border-border/60">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-28">Size</TableHead>
              <TableHead className="w-44">Uploaded</TableHead>
              <TableHead className="w-28 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {files.map((f) => {
              const viewHref = apiURL(
                `/f/${f.fileId}/${encodeURIComponent(f.fileName)}`,
              );
              const downloadHref = `${viewHref}?download=1`;
              return (
                <TableRow
                  key={f.fileId}
                  data-state={selectedId === f.fileId ? "selected" : undefined}
                  className={cn(selectedId === f.fileId && "bg-muted/50")}
                >
                  <TableCell className="max-w-0 truncate font-medium">
                    <button
                      type="button"
                      className="truncate text-left hover:underline"
                      onClick={() =>
                        setSelectedId((id) =>
                          id === f.fileId ? null : f.fileId,
                        )
                      }
                    >
                      {f.fileName}
                    </button>
                  </TableCell>
                  <TableCell className="tabular-nums text-muted-foreground">
                    {formatBytes(f.fileSize)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(f.createdAt)}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="inline-flex items-center gap-0.5">
                      <a
                        href={viewHref}
                        target="_blank"
                        rel="noreferrer"
                        className={buttonVariants({
                          variant: "ghost",
                          size: "icon-sm",
                        })}
                        aria-label={`Open ${f.fileName} in new tab`}
                      >
                        <ExternalLink aria-hidden />
                      </a>
                      <a
                        href={downloadHref}
                        className={buttonVariants({
                          variant: "ghost",
                          size: "icon-sm",
                        })}
                        aria-label={`Download ${f.fileName}`}
                      >
                        <Download aria-hidden />
                      </a>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Remove ${f.fileName} from list`}
                        onClick={() => {
                          if (selectedId === f.fileId) setSelectedId(null);
                          removeLocalFile(f.fileId);
                        }}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>

      {selected && (
        <FileDetails
          key={selected.fileId}
          local={selected}
          onClose={() => setSelectedId(null)}
        />
      )}
    </div>
  );
}

function FileDetails({
  local,
  onClose,
}: {
  local: LocalFile;
  onClose: () => void;
}) {
  const [meta, setMeta] = useState<FileMeta | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const ac = new AbortController();
    fetchFileMeta(local.fileId, { signal: ac.signal })
      .then((m) => {
        if (!ac.signal.aborted) setMeta(m);
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
  }, [local.fileId]);

  const name = meta?.fileName ?? local.fileName;
  const size = meta?.fileSize ?? local.fileSize;
  const created = meta?.createdAt ?? local.createdAt;
  const chunks =
    meta && meta.chunkSize > 0
      ? Math.ceil(meta.fileSize / meta.chunkSize)
      : null;

  return (
    <Card>
      <CardContent className="flex flex-col gap-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 className="truncate text-lg font-semibold tracking-tight">
              {name}
            </h2>
            <p className="text-sm text-muted-foreground">File metadata</p>
          </div>
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label="Close details"
            onClick={onClose}
          >
            <X aria-hidden />
          </Button>
        </div>

        {loading && (
          <div className="flex flex-col gap-2">
            <Skeleton className="h-4 w-2/3" />
            <Skeleton className="h-4 w-1/2" />
            <Skeleton className="h-4 w-3/5" />
          </div>
        )}

        {error && (
          <p className="text-sm text-destructive" role="alert">
            {error}
            <span className="mt-1 block text-muted-foreground">
              Showing local list data only.
            </span>
          </p>
        )}

        <dl className="grid gap-3 text-sm sm:grid-cols-2">
          <MetaItem label="File ID" value={local.fileId} mono />
          <MetaItem label="Size" value={formatBytes(size)} />
          <MetaItem label="Uploaded" value={formatDate(created)} />
          {meta && (
            <MetaItem
              label="Chunk size"
              value={formatBytes(meta.chunkSize)}
            />
          )}
          {chunks != null && (
            <MetaItem
              label="Chunks"
              value={`${chunks}`}
            />
          )}
        </dl>

        <div className="flex flex-col gap-3">
          <LinkRow label="Share link" href={local.longURL} />
          <LinkRow label="Direct download" href={local.longDownloadURL} />
        </div>

        <div className="flex flex-wrap gap-2">
          <a
            href={local.longURL}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            Open
          </a>
          <a
            href={local.longDownloadURL}
            className={buttonVariants({ variant: "default", size: "sm" })}
          >
            <Download aria-hidden /> Download
          </a>
        </div>
      </CardContent>
    </Card>
  );
}

function MetaItem({
  label,
  value,
  mono,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className={cn("mt-0.5 truncate", mono && "font-mono text-xs")}>
        {value}
      </dd>
    </div>
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
