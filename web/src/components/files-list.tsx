"use client";

import {
  Download,
  ExternalLink,
  Eye,
  EyeOff,
  FolderOpen,
  KeyRound,
  MoreHorizontal,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useEffect, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { TokenRevealPanel, type TokenReveal } from "@/components/token-reveal";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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

function OwnedFilesList() {
  const [files, setFiles] = useState<OwnedFile[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [reveal, setReveal] = useState<TokenReveal | null>(null);
  const [confirmDelete, setConfirmDelete] = useState<OwnedFile | null>(null);

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

  async function runAction(
    fileId: string,
    action: () => Promise<void>,
  ): Promise<void> {
    setBusyId(fileId);
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
      setBusyId(null);
    }
  }

  async function onVisibility(f: OwnedFile, visibility: Visibility) {
    await runAction(f.fileId, async () => {
      const res = await setFileVisibility(f.fileId, visibility);
      setFiles(
        (prev) =>
          prev?.map((row) =>
            row.fileId === f.fileId
              ? {
                  ...row,
                  visibility: res.visibility ?? visibility,
                }
              : row,
          ) ?? null,
      );
      if (visibility === "private" && res.accessToken) {
        setReveal({
          fileId: f.fileId,
          fileName: f.fileName,
          accessToken: res.accessToken,
        });
      } else if (visibility === "public") {
        setReveal(null);
        toast.success("File is public — previous private token invalidated");
      }
    });
  }

  async function onRotate(f: OwnedFile) {
    await runAction(f.fileId, async () => {
      const res = await rotateAccessToken(f.fileId);
      if (!res.accessToken) {
        toast.error("Server did not return a new token");
        return;
      }
      setReveal({
        fileId: f.fileId,
        fileName: f.fileName,
        accessToken: res.accessToken,
      });
      toast.success("Access token rotated");
    });
  }

  async function onDeleteConfirmed(f: OwnedFile) {
    await runAction(f.fileId, async () => {
      await deleteFile(f.fileId);
      setFiles((prev) => prev?.filter((row) => row.fileId !== f.fileId) ?? null);
      setConfirmDelete(null);
      if (reveal?.fileId === f.fileId) setReveal(null);
      toast.success("File deleted from DisCloud");
    });
  }

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

      {reveal && (
        <TokenRevealPanel reveal={reveal} onDismiss={() => setReveal(null)} />
      )}

      {confirmDelete && (
        <div
          role="alertdialog"
          aria-labelledby="delete-title"
          className="rounded-xl border border-destructive/40 bg-destructive/5 p-4"
        >
          <p id="delete-title" className="font-medium">
            Delete {confirmDelete.fileName}?
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Share and download links stop working immediately. Discord
            attachments are not deleted.
          </p>
          <div className="mt-3 flex flex-wrap gap-2">
            <Button
              type="button"
              variant="destructive"
              size="sm"
              disabled={busyId === confirmDelete.fileId}
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
        <p className="text-sm text-destructive" role="alert">
          {error}
        </p>
      )}

      {files.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
          <FolderOpen className="size-6 text-muted-foreground" aria-hidden />
          <h2 className="font-semibold">No files yet</h2>
          <p className="max-w-sm text-sm text-muted-foreground">
            Upload while signed in to see files here with visibility and delete
            controls.
          </p>
          <Link href="/" className={buttonVariants({ size: "sm" })}>
            Upload
          </Link>
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border border-border/60">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead className="w-24">Size</TableHead>
                <TableHead className="w-28">Visibility</TableHead>
                <TableHead className="w-40">Expires</TableHead>
                <TableHead className="w-12 text-right">
                  <span className="sr-only">Actions</span>
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {files.map((f) => {
                const busy = busyId === f.fileId;
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
                  <TableRow key={f.fileId}>
                    <TableCell className="max-w-0 truncate font-medium">
                      <Link
                        href={buildInspectPath(f.fileId)}
                        className="truncate hover:underline"
                      >
                        {f.fileName}
                      </Link>
                      <p className="truncate font-mono text-xs font-normal text-muted-foreground">
                        {formatDate(f.createdAt)}
                      </p>
                    </TableCell>
                    <TableCell className="tabular-nums text-muted-foreground">
                      {formatBytes(f.fileSize)}
                    </TableCell>
                    <TableCell>
                      <Badge
                        variant={
                          f.visibility === "private" ? "outline" : "secondary"
                        }
                      >
                        {f.visibility}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {formatDate(f.expiresAt)}
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon-sm"
                            />
                          }
                          disabled={busy}
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
                                <a
                                  href={viewHref}
                                  target="_blank"
                                  rel="noreferrer"
                                />
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
                          </DropdownMenuGroup>
                          <DropdownMenuSeparator />
                          <DropdownMenuGroup>
                            {f.visibility === "public" ? (
                              <DropdownMenuItem
                                disabled={busy}
                                onClick={() => void onVisibility(f, "private")}
                              >
                                <EyeOff aria-hidden />
                                Make private
                              </DropdownMenuItem>
                            ) : (
                              <>
                                <DropdownMenuItem
                                  disabled={busy}
                                  onClick={() => void onVisibility(f, "public")}
                                >
                                  <Eye aria-hidden />
                                  Make public
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  disabled={busy}
                                  onClick={() => void onRotate(f)}
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
                              disabled={busy}
                              onClick={() => setConfirmDelete(f)}
                            >
                              <Trash2 aria-hidden />
                              Delete
                            </DropdownMenuItem>
                          </DropdownMenuGroup>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
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

  if (files.length === 0) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
        <FolderOpen className="size-6 text-muted-foreground" aria-hidden />
        <h1 className="font-semibold">No files yet</h1>
        <p className="max-w-sm text-sm text-muted-foreground">
          Anonymous uploads in this browser are listed here.{" "}
          <Link href="/signin" className="underline-offset-2 hover:underline">
            Sign in
          </Link>{" "}
          to own files on the server (longer retention, visibility, delete).
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
        the files on the server.{" "}
        <Link href="/signin" className="underline-offset-2 hover:underline">
          Sign in
        </Link>{" "}
        for server-side My files with delete and visibility.
      </p>
      <div className="overflow-hidden rounded-xl border border-border/60">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-28">Size</TableHead>
              <TableHead className="w-44">Uploaded</TableHead>
              <TableHead className="w-12 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {files.map((f) => {
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
                <TableRow key={f.fileId}>
                  <TableCell className="max-w-0 truncate font-medium">
                    <Link
                      href={buildInspectPath(f.fileId)}
                      className="truncate hover:underline"
                    >
                      {f.fileName}
                    </Link>
                  </TableCell>
                  <TableCell className="tabular-nums text-muted-foreground">
                    {formatBytes(f.fileSize)}
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(f.createdAt)}
                  </TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                          />
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
                              <a
                                href={viewHref}
                                target="_blank"
                                rel="noreferrer"
                              />
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
                        </DropdownMenuGroup>
                        <DropdownMenuSeparator />
                        <DropdownMenuGroup>
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => removeLocalFile(f.fileId)}
                          >
                            <Trash2 aria-hidden />
                            Remove from list
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
