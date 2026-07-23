"use client";

import { Download, ExternalLink, RefreshCw, Share2 } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { CopyButton } from "@/components/copy-button";
import { ShareQR } from "@/components/share-qr";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  ApiError,
  buildFileURL,
  buildInspectPath,
  fetchFileInspect,
  revokeFile,
  unlockFile,
  updateFileShare,
  type FileInspect,
  type ShareMode,
} from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";

function mimeKind(name: string): "image" | "video" | "other" {
  const ext = name.split(".").pop()?.toLowerCase() ?? "";
  if (["png", "jpg", "jpeg", "gif", "webp", "avif"].includes(ext)) {
    return "image";
  }
  if (["mp4", "webm", "ogg", "mov"].includes(ext)) return "video";
  return "other";
}

export function FileInspectPanel({
  fileId,
  accessToken,
}: {
  fileId: string;
  accessToken?: string;
}) {
  const [data, setData] = useState<FileInspect | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [needPassword, setNeedPassword] = useState(false);
  const [password, setPassword] = useState("");
  const [unlockBusy, setUnlockBusy] = useState(false);
  const [showPreview, setShowPreview] = useState(false);

  const load = useCallback(() => {
    const ac = new AbortController();
    void Promise.resolve().then(() => {
      if (!ac.signal.aborted) setLoading(true);
    });
    fetchFileInspect(fileId, { signal: ac.signal, token: accessToken })
      .then((info) => {
        if (ac.signal.aborted) return;
        setData(info);
        setError(null);
        setNeedPassword(false);
      })
      .catch((err: unknown) => {
        if (ac.signal.aborted) return;
        if (err instanceof DOMException && err.name === "AbortError") return;
        if (
          err instanceof ApiError &&
          err.status === 401 &&
          (err.code === "password_required" || err.code === "password_invalid")
        ) {
          setNeedPassword(true);
          setError(null);
          setData(null);
          return;
        }
        setError(err instanceof Error ? err.message : "Failed to load");
      })
      .finally(() => {
        if (!ac.signal.aborted) setLoading(false);
      });
    return () => ac.abort();
  }, [fileId, accessToken]);

  useEffect(() => load(), [load]);

  const links = useMemo(() => {
    if (!data) return null;
    const token = accessToken;
    return {
      view: token
        ? buildFileURL({
          fileId: data.fileId,
          fileName: data.fileName,
          token,
        })
        : data.longURL,
      download: token
        ? buildFileURL({
          fileId: data.fileId,
          fileName: data.fileName,
          download: true,
          token,
        })
        : data.longDownloadURL,
      preview: token
        ? buildFileURL({ fileId: data.fileId, token })
        : data.url,
      inspect:
        typeof window !== "undefined"
          ? `${window.location.origin}${buildInspectPath(data.fileId, token)}`
          : buildInspectPath(data.fileId, token),
    };
  }, [data, accessToken]);

  async function onUnlock(e: React.FormEvent) {
    e.preventDefault();
    setUnlockBusy(true);
    try {
      await unlockFile(fileId, password, accessToken);
      setPassword("");
      setNeedPassword(false);
      load();
      toast.success("File unlocked");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unlock failed");
    } finally {
      setUnlockBusy(false);
    }
  }

  if (loading && !data && !needPassword) {
    return (
      <div className="flex flex-col gap-4">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-24 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    );
  }

  if (needPassword) {
    return (
      <div className="flex min-h-[50vh] items-center justify-center">
        <Card className="w-full max-w-md">
          <CardContent className="flex flex-col gap-4 py-6">
            <div>
              <h1 className="text-lg font-semibold">Password required</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                This share is password-protected. Enter the password to continue.
              </p>
            </div>
            <form onSubmit={onUnlock} className="flex flex-col gap-3">
              <Field>
                <FieldLabel htmlFor="share-password">Password</FieldLabel>
                <Input
                  id="share-password"
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(ev) => setPassword(ev.target.value)}
                  required
                />
              </Field>
              <Button type="submit" disabled={unlockBusy || !password}>
                {unlockBusy ? "Unlocking…" : "Unlock"}
              </Button>
            </form>
          </CardContent>
        </Card>
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

  if (!data || !links) return null;

  const kind = mimeKind(data.fileName);
  const downloadsDisabled = data.shareMode === "view";

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
          <div className="mt-2 flex flex-wrap items-center gap-1.5">
            <Badge
              variant={data.status === "reused" ? "outline" : "secondary"}
            >
              {data.status === "reused" ? "Reused" : "Ready"}
            </Badge>
            {data.visibility && (
              <Badge variant="secondary">{data.visibility}</Badge>
            )}
            {data.passwordProtected && (
              <Badge variant="outline">Password</Badge>
            )}
            {data.shareMode === "view" && (
              <Badge variant="outline">View only</Badge>
            )}
            {data.expiresAt && (
              <span className="text-xs text-muted-foreground">
                Expires {formatDate(data.expiresAt)}
              </span>
            )}
            {accessToken && (
              <Badge variant="outline">Private link</Badge>
            )}
          </div>
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
                    url: links.view,
                  });
                  return;
                }
                await navigator.clipboard.writeText(links.view);
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
            href={links.view}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ variant: "outline", size: "sm" })}
          >
            <ExternalLink aria-hidden /> Open
          </a>
          {!downloadsDisabled && (
            <a
              href={links.download}
              className={buttonVariants({ variant: "default", size: "sm" })}
            >
              <Download aria-hidden /> Download
            </a>
          )}
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => load()}
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

      {data.ownedByCurrentUser &&
        data.maxDownloads != null &&
        data.downloadCount != null && (
          <p className="text-xs text-muted-foreground">
            Share downloads: {data.downloadCount} / {data.maxDownloads}
          </p>
        )}

      <p className="text-xs text-muted-foreground">
        {data.chunkCount} chunk{data.chunkCount === 1 ? "" : "s"} ×{" "}
        {formatBytes(data.chunkSize)} · Uploaded {formatDate(data.createdAt)}
        {data.lastAccessAt
          ? ` · Last access ${formatDate(data.lastAccessAt)}`
          : ""}
      </p>

      {data.sha256 && (
        <div className="flex min-w-0 items-center gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2">
          <div className="min-w-0 flex-1">
            <p className="text-xs font-medium text-muted-foreground">SHA-256</p>
            <p className="truncate font-mono text-xs">{data.sha256}</p>
          </div>
          <CopyButton value={data.sha256} label="Copy SHA-256" />
        </div>
      )}

      {data.ownedByCurrentUser && (
        <ShareSettingsCard
          fileId={data.fileId}
          data={data}
          onUpdated={(next) => setData(next)}
        />
      )}

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
                  src={links.preview}
                  alt={data.fileName}
                  className="max-h-96 w-full rounded-lg object-contain bg-muted"
                />
              ) : (
                <video
                  src={links.preview}
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
        <ShareQR value={links.view} />
        <div className="flex flex-col gap-3">
          <LinkRow label="Share link" href={links.view} />
          {!downloadsDisabled && (
            <LinkRow label="Download" href={links.download} />
          )}
          <LinkRow label="Inspect" href={links.inspect} />
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

function ShareSettingsCard({
  fileId,
  data,
  onUpdated,
}: {
  fileId: string;
  data: FileInspect;
  onUpdated: (next: FileInspect) => void;
}) {
  const [password, setPassword] = useState("");
  const [expiresLocal, setExpiresLocal] = useState(() =>
    data.expiresAt ? toLocalInput(data.expiresAt) : "",
  );
  const [maxDownloads, setMaxDownloads] = useState(
    data.maxDownloads != null ? String(data.maxDownloads) : "",
  );
  const [shareMode, setShareMode] = useState<ShareMode>(
    data.shareMode ?? "download",
  );
  const [busy, setBusy] = useState(false);

  async function save(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    try {
      const patch: Parameters<typeof updateFileShare>[1] = {
        shareMode,
      };
      if (password) patch.password = password;
      if (expiresLocal) {
        patch.expiresAt = new Date(expiresLocal).toISOString();
      }
      if (maxDownloads.trim() === "") {
        patch.maxDownloads = 0;
      } else {
        const n = Number(maxDownloads);
        if (!Number.isFinite(n) || n < 0) {
          toast.error("Max downloads must be a positive number or empty");
          return;
        }
        patch.maxDownloads = n === 0 ? 0 : Math.floor(n);
      }
      const res = await updateFileShare(fileId, patch);
      onUpdated({
        ...data,
        expiresAt: res.expiresAt ?? data.expiresAt,
        passwordProtected: res.passwordProtected ?? data.passwordProtected,
        shareMode: res.shareMode ?? shareMode,
        maxDownloads: res.maxDownloads ?? null,
        downloadCount: res.downloadCount ?? data.downloadCount,
      });
      setPassword("");
      toast.success("Share settings saved");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Save failed");
    } finally {
      setBusy(false);
    }
  }

  async function clearPassword() {
    setBusy(true);
    try {
      const res = await updateFileShare(fileId, { password: "" });
      onUpdated({
        ...data,
        passwordProtected: res.passwordProtected ?? false,
      });
      toast.success("Password cleared");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Clear failed");
    } finally {
      setBusy(false);
    }
  }

  async function onRevoke() {
    if (!window.confirm("Revoke this share? The link will stop working.")) {
      return;
    }
    setBusy(true);
    try {
      await revokeFile(fileId);
      toast.success("Share revoked");
      window.location.href = "/files";
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Revoke failed");
      setBusy(false);
    }
  }

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 py-5">
        <div>
          <h2 className="text-sm font-semibold">Share settings</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Password, expiry, download cap, and mode. Revoke force-expires the
            link without deleting Discord storage.
          </p>
        </div>
        <form onSubmit={save}>
          <FieldGroup className="gap-3">
            <Field>
              <FieldLabel htmlFor="share-new-password">
                {data.passwordProtected
                  ? "Set new password"
                  : "Password (optional)"}
              </FieldLabel>
              <Input
                id="share-new-password"
                type="password"
                autoComplete="new-password"
                value={password}
                onChange={(ev) => setPassword(ev.target.value)}
                placeholder={
                  data.passwordProtected ? "Leave blank to keep" : "Min 8 chars"
                }
              />
            </Field>
            {data.passwordProtected && (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={busy}
                onClick={() => void clearPassword()}
              >
                Clear password
              </Button>
            )}
            <Field>
              <FieldLabel htmlFor="share-expires">Expires</FieldLabel>
              <Input
                id="share-expires"
                type="datetime-local"
                value={expiresLocal}
                onChange={(ev) => setExpiresLocal(ev.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="share-max-dl">
                Max downloads (empty = unlimited)
              </FieldLabel>
              <Input
                id="share-max-dl"
                type="number"
                min={0}
                value={maxDownloads}
                onChange={(ev) => setMaxDownloads(ev.target.value)}
              />
            </Field>
            <Field>
              <FieldLabel>Mode</FieldLabel>
              <Select
                value={shareMode}
                onValueChange={(v) => {
                  if (v === "view" || v === "download") setShareMode(v);
                }}
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="download">View + download</SelectItem>
                  <SelectItem value="view">View only</SelectItem>
                </SelectContent>
              </Select>
            </Field>
            <div className="flex flex-wrap gap-2 pt-1">
              <Button type="submit" size="sm" disabled={busy}>
                Save
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={busy}
                onClick={() => void onRevoke()}
              >
                Revoke link
              </Button>
            </div>
          </FieldGroup>
        </form>
      </CardContent>
    </Card>
  );
}

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
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
