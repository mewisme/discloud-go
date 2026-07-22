"use client";

import type { ColumnDef } from "@tanstack/react-table";
import {
  Download,
  ExternalLink,
  Eye,
  EyeOff,
  FolderOpen,
  KeyRound,
  MoreHorizontal,
  Search,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { TokenRevealPanel, type TokenReveal } from "@/components/token-reveal";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { DataTable, selectColumn } from "@/components/ui/data-table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Skeleton } from "@/components/ui/skeleton";
import {
  ApiError,
  buildFileURL,
  buildInspectPath,
  deleteFile,
  listMyFiles,
  rotateAccessToken,
  setFileVisibility,
  type OwnedFile,
  type Visibility,
} from "@/lib/api";
import {
  ensureAuth,
  getAuthServerSnapshot,
  getAuthSnapshot,
  subscribeAuth,
} from "@/lib/auth";
import { formatBytes, formatDate } from "@/lib/format";
import {
  getLocalFilesServerSnapshot,
  getLocalFilesSnapshot,
  removeLocalFile,
  subscribeLocalFiles,
  type LocalFile,
} from "@/lib/local-files";

export function FilesList() {
  const user = useSyncExternalStore(
    subscribeAuth,
    getAuthSnapshot,
    getAuthServerSnapshot,
  );

  useEffect(() => {
    void ensureAuth();
  }, []);

  if (user === undefined) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (user) return <OwnedFilesList />;
  return <LocalFilesList />;
}

type OwnedActions = {
  busy: boolean;
  onVisibility: (f: OwnedFile, visibility: Visibility) => void;
  onRotate: (f: OwnedFile) => void;
  onDelete: (f: OwnedFile) => void;
};

function ownedColumns(actions: OwnedActions): ColumnDef<OwnedFile>[] {
  return [
    selectColumn<OwnedFile>(),
    {
      accessorKey: "fileName",
      header: "Name",
      meta: { className: "max-w-0 truncate font-medium" },
      cell: ({ row }) => {
        const f = row.original;
        return (
          <>
            <Link
              href={buildInspectPath(f.fileId)}
              className="truncate hover:underline"
            >
              {f.fileName}
            </Link>
            <p className="truncate font-mono text-xs font-normal text-muted-foreground">
              {formatDate(f.createdAt)}
            </p>
          </>
        );
      },
    },
    {
      accessorKey: "fileSize",
      header: "Size",
      meta: {
        headerClassName: "w-24",
        className: "w-24 tabular-nums text-muted-foreground",
      },
      cell: ({ row }) => formatBytes(row.original.fileSize),
    },
    {
      accessorKey: "status",
      header: "Status",
      meta: { headerClassName: "w-28", className: "w-28" },
      cell: ({ row }) => {
        const status = row.original.status ?? "ready";
        return (
          <Badge variant={status === "duplicate" ? "outline" : "secondary"}>
            {status}
          </Badge>
        );
      },
    },
    {
      accessorKey: "visibility",
      header: "Visibility",
      meta: { headerClassName: "w-28", className: "w-28" },
      cell: ({ row }) => {
        const visibility = row.original.visibility;
        return (
          <Badge variant={visibility === "private" ? "outline" : "secondary"}>
            {visibility}
          </Badge>
        );
      },
    },
    {
      accessorKey: "expiresAt",
      header: "Expires",
      meta: {
        headerClassName: "w-40",
        className: "w-40 text-muted-foreground",
      },
      cell: ({ row }) => formatDate(row.original.expiresAt),
    },
    {
      id: "actions",
      header: () => <span className="sr-only">Actions</span>,
      meta: { headerClassName: "w-12 text-right", className: "text-right" },
      cell: ({ row }) => {
        const f = row.original;
        const viewHref = buildFileURL({
          fileId: f.fileId,
          fileName: f.fileName,
        });
        const downloadHref = buildFileURL({
          fileId: f.fileId,
          fileName: f.fileName,
          download: true,
        });
        return (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button type="button" variant="ghost" size="icon-sm" />
              }
              disabled={actions.busy}
              aria-label={`Actions for ${f.fileName}`}
            >
              <MoreHorizontal aria-hidden />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-44">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={
                    <a href={viewHref} target="_blank" rel="noreferrer" />
                  }
                >
                  <ExternalLink aria-hidden />
                  Open
                </DropdownMenuItem>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={<a href={downloadHref} />}
                >
                  <Download aria-hidden />
                  Download
                </DropdownMenuItem>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={<Link href={buildInspectPath(f.fileId)} />}
                >
                  <Search aria-hidden />
                  Inspect
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                {f.visibility === "public" ? (
                  <DropdownMenuItem
                    disabled={actions.busy}
                    onClick={() => actions.onVisibility(f, "private")}
                  >
                    <EyeOff aria-hidden />
                    Make private
                  </DropdownMenuItem>
                ) : (
                  <>
                    <DropdownMenuItem
                      disabled={actions.busy}
                      onClick={() => actions.onVisibility(f, "public")}
                    >
                      <Eye aria-hidden />
                      Make public
                    </DropdownMenuItem>
                    <DropdownMenuItem
                      disabled={actions.busy}
                      onClick={() => actions.onRotate(f)}
                    >
                      <KeyRound aria-hidden />
                      Rotate token
                    </DropdownMenuItem>
                  </>
                )}
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  variant="destructive"
                  disabled={actions.busy}
                  onClick={() => actions.onDelete(f)}
                >
                  <Trash2 aria-hidden />
                  Delete
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        );
      },
    },
  ];
}

function localColumns(
  onBulkDeleteReady: (ids: string[]) => void,
): ColumnDef<LocalFile>[] {
  return [
    selectColumn<LocalFile>(),
    {
      accessorKey: "fileName",
      header: "Name",
      meta: { className: "max-w-0 truncate font-medium" },
      cell: ({ row }) => {
        const f = row.original;
        return (
          <Link
            href={buildInspectPath(f.fileId)}
            className="truncate hover:underline"
          >
            {f.fileName}
          </Link>
        );
      },
    },
    {
      accessorKey: "fileSize",
      header: "Size",
      meta: {
        headerClassName: "w-28",
        className: "w-28 tabular-nums text-muted-foreground",
      },
      cell: ({ row }) => formatBytes(row.original.fileSize),
    },
    {
      accessorKey: "createdAt",
      header: "Uploaded",
      meta: {
        headerClassName: "w-44",
        className: "w-44 text-muted-foreground",
      },
      cell: ({ row }) => formatDate(row.original.createdAt),
    },
    {
      id: "actions",
      header: () => <span className="sr-only">Actions</span>,
      meta: { headerClassName: "w-12 text-right", className: "text-right" },
      cell: ({ row }) => {
        const f = row.original;
        const viewHref = buildFileURL({
          fileId: f.fileId,
          fileName: f.fileName,
        });
        const downloadHref = buildFileURL({
          fileId: f.fileId,
          fileName: f.fileName,
          download: true,
        });
        return (
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button type="button" variant="ghost" size="icon-sm" />
              }
              aria-label={`Actions for ${f.fileName}`}
            >
              <MoreHorizontal aria-hidden />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-44">
              <DropdownMenuGroup>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={
                    <a href={viewHref} target="_blank" rel="noreferrer" />
                  }
                >
                  <ExternalLink aria-hidden />
                  Open
                </DropdownMenuItem>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={<a href={downloadHref} />}
                >
                  <Download aria-hidden />
                  Download
                </DropdownMenuItem>
                <DropdownMenuItem
                  nativeButton={false}
                  closeOnClick
                  render={<Link href={buildInspectPath(f.fileId)} />}
                >
                  <Search aria-hidden />
                  Inspect
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => onBulkDeleteReady([f.fileId])}
                >
                  <Trash2 aria-hidden />
                  Remove from list
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        );
      },
    },
  ];
}

function OwnedFilesList() {
  const [files, setFiles] = useState<OwnedFile[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [reveals, setReveals] = useState<TokenReveal[]>([]);
  const [confirmDelete, setConfirmDelete] = useState<OwnedFile[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listMyFiles({ limit: 100, offset: 0 })
      .then((res) => {
        if (cancelled) return;
        setError(null);
        setFiles(res.files ?? []);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setFiles([]);
        setError(err instanceof Error ? err.message : "Could not load files");
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function withBusy(action: () => Promise<void>): Promise<void> {
    setBusy(true);
    try {
      await action();
    } catch (err) {
      const msg =
        err instanceof ApiError
          ? err.message
          : err instanceof Error
            ? err.message
            : "Action failed";
      toast.error(msg);
    } finally {
      setBusy(false);
    }
  }

  async function applyVisibility(targets: OwnedFile[], visibility: Visibility) {
    await withBusy(async () => {
      const nextReveals: TokenReveal[] = [];
      let ok = 0;
      try {
        for (const f of targets) {
          if (f.visibility === visibility) {
            ok++;
            continue;
          }
          const res = await setFileVisibility(f.fileId, visibility);
          setFiles(
            (prev) =>
              prev?.map((row) =>
                row.fileId === f.fileId
                  ? { ...row, visibility: res.visibility ?? visibility }
                  : row,
              ) ?? null,
          );
          if (visibility === "private" && res.accessToken) {
            nextReveals.push({
              fileId: f.fileId,
              fileName: f.fileName,
              accessToken: res.accessToken,
            });
          }
          ok++;
        }
        if (visibility === "public") {
          setReveals([]);
          toast.success(
            ok === 1
              ? "File is public — previous private token invalidated"
              : `${ok} files are public — private tokens invalidated`,
          );
        } else if (nextReveals.length === 0) {
          toast.success(
            ok === 1 ? "File is private" : `${ok} files set to private`,
          );
        }
      } finally {
        if (visibility === "private" && nextReveals.length > 0) {
          setReveals(nextReveals);
        }
      }
    });
  }

  async function onRotate(f: OwnedFile) {
    await withBusy(async () => {
      const res = await rotateAccessToken(f.fileId);
      if (!res.accessToken) {
        toast.error("Server did not return a new token");
        return;
      }
      setReveals([
        {
          fileId: f.fileId,
          fileName: f.fileName,
          accessToken: res.accessToken,
        },
      ]);
      toast.success("Access token rotated");
    });
  }

  async function onDeleteConfirmed(targets: OwnedFile[]) {
    await withBusy(async () => {
      const ids = new Set(targets.map((t) => t.fileId));
      for (const f of targets) {
        await deleteFile(f.fileId);
      }
      setFiles((prev) => prev?.filter((row) => !ids.has(row.fileId)) ?? null);
      setConfirmDelete(null);
      setReveals((prev) => prev.filter((r) => !ids.has(r.fileId)));
      toast.success(
        targets.length === 1
          ? "File deleted from DisCloud"
          : `${targets.length} files deleted from DisCloud`,
      );
    });
  }

  const columns = ownedColumns({
    busy,
    onVisibility: (f, visibility) => void applyVisibility([f], visibility),
    onRotate: (f) => void onRotate(f),
    onDelete: (f) => setConfirmDelete([f]),
  });

  if (files === null) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-baseline justify-between gap-3">
        <h1 className="text-2xl font-semibold tracking-tight">My files</h1>
        <Badge variant="secondary">
          {files.length} file{files.length === 1 ? "" : "s"}
        </Badge>
      </div>
      <p className="text-sm text-muted-foreground">
        Files you own on the server. Public by default; private files need a
        token. Delete removes database records only — Discord attachments stay.
      </p>

      {reveals.length > 0 && (
        <TokenRevealPanel
          reveals={reveals}
          onDismiss={() => setReveals([])}
        />
      )}

      {confirmDelete && confirmDelete.length > 0 && (
        <div
          role="alertdialog"
          aria-labelledby="delete-title"
          className="rounded-xl border border-destructive/40 bg-destructive/5 p-4"
        >
          <p id="delete-title" className="font-medium">
            {confirmDelete.length === 1
              ? `Delete ${confirmDelete[0].fileName}?`
              : `Delete ${confirmDelete.length} files?`}
          </p>
          {confirmDelete.length > 1 && (
            <ul className="mt-2 max-h-32 list-inside list-disc overflow-y-auto text-sm text-muted-foreground">
              {confirmDelete.map((f) => (
                <li key={f.fileId} className="truncate">
                  {f.fileName}
                </li>
              ))}
            </ul>
          )}
          <p className="mt-1 text-sm text-muted-foreground">
            Share and download links stop working immediately. Discord
            attachments are not deleted.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={busy}
              onClick={() => void onDeleteConfirmed(confirmDelete)}
            >
              Delete permanently
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setConfirmDelete(null)}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}

      {files.length === 0 ? (
        <Empty className="border border-dashed border-border py-16">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <FolderOpen aria-hidden />
            </EmptyMedia>
            <EmptyTitle>No files yet</EmptyTitle>
            <EmptyDescription>
              Upload while signed in to see files here with visibility and delete
              controls.
            </EmptyDescription>
          </EmptyHeader>
          <EmptyContent>
            <Link href="/" className={buttonVariants({ size: "sm" })}>
              Upload
            </Link>
          </EmptyContent>
        </Empty>
      ) : (
        <DataTable
          columns={columns}
          data={files}
          getRowId={(row) => row.fileId}
          enableRowSelection
          toolbar={({ selected, clearSelection }) => (
            <div className="flex flex-wrap items-center gap-2 rounded-xl border border-border/60 bg-muted/40 px-3 py-2">
              <span className="mr-1 text-sm text-muted-foreground">
                {selected.length} selected
              </span>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() => {
                  void applyVisibility(selected, "private").then(clearSelection);
                }}
              >
                <EyeOff aria-hidden />
                Make private
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() => {
                  void applyVisibility(selected, "public").then(clearSelection);
                }}
              >
                <Eye aria-hidden />
                Make public
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={busy}
                onClick={() => {
                  setConfirmDelete(selected);
                  clearSelection();
                }}
              >
                <Trash2 aria-hidden />
                Delete
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={clearSelection}
              >
                Clear
              </Button>
            </div>
          )}
        />
      )}
    </div>
  );
}

function LocalFilesList() {
  const files = useSyncExternalStore(
    subscribeLocalFiles,
    getLocalFilesSnapshot,
    getLocalFilesServerSnapshot,
  );

  const columns = useMemo(
    () =>
      localColumns((ids) => {
        for (const id of ids) removeLocalFile(id);
      }),
    [],
  );

  if (files.length === 0) {
    return (
      <Empty className="border border-dashed border-border py-16">
        <EmptyHeader>
          <EmptyMedia variant="icon">
            <FolderOpen aria-hidden />
          </EmptyMedia>
          <EmptyTitle>No files yet</EmptyTitle>
          <EmptyDescription>
            Anonymous uploads in this browser are listed here.{" "}
            <Link href="/signin" className="underline-offset-2 hover:underline">
              Sign in
            </Link>{" "}
            to own files on the server (longer retention, visibility, delete).
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
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
        the files on the server.{" "}
        <Link href="/signin" className="underline-offset-2 hover:underline">
          Sign in
        </Link>{" "}
        for server-side My files with delete and visibility.
      </p>
      <DataTable
        columns={columns}
        data={files}
        getRowId={(row) => row.fileId}
        enableRowSelection
        toolbar={({ selected, clearSelection }) => (
          <div className="flex flex-wrap items-center gap-2 rounded-xl border border-border/60 bg-muted/40 px-3 py-2">
            <span className="mr-1 text-sm text-muted-foreground">
              {selected.length} selected
            </span>
            <Button
              type="button"
              variant="destructive"
              size="sm"
              onClick={() => {
                for (const f of selected) removeLocalFile(f.fileId);
                clearSelection();
              }}
            >
              <Trash2 aria-hidden />
              Remove from list
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              onClick={clearSelection}
            >
              Clear
            </Button>
          </div>
        )}
      />
    </div>
  );
}
