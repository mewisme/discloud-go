"use client";

import {
  CloudUpload,
  Download,
  ExternalLink,
  FileIcon,
  Loader2,
  RotateCcw,
  Search,
  Share2,
  X,
} from "lucide-react";
import Link from "next/link";
import { useCallback, useRef, useState, useSyncExternalStore } from "react";
import { toast } from "sonner";

import { CopyButton } from "@/components/copy-button";
import { ShareQR } from "@/components/share-qr";
import { TokenRevealPanel } from "@/components/token-reveal";
import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@/components/ui/progress";
import { buildInspectPath, type UploadResult } from "@/lib/api";
import { formatBytes, formatDate, formatSpeed } from "@/lib/format";
import {
  cancelUpload,
  dismissAllResults,
  dismissResult,
  enqueue,
  getState,
  removeQueued,
  subscribe,
} from "@/lib/upload-manager";
import { cn } from "@/lib/utils";

export function Uploader() {
  const state = useSyncExternalStore(subscribe, getState, getState);
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const addFiles = useCallback((list: FileList | File[] | null) => {
    if (!list?.length) return;
    enqueue(list);
  }, []);

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      addFiles(e.dataTransfer.files);
    },
    [addFiles],
  );

  const busy = Boolean(state.uploading) || state.queue.length > 0;
  const processing = state.uploading?.phase === "processing";

  return (
    <div className="flex w-full flex-col gap-4">
      <div
        role="button"
        tabIndex={0}
        aria-label="Upload files"
        onClick={() => inputRef.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            inputRef.current?.click();
          }
        }}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={onDrop}
        className={cn(
          "flex w-full cursor-pointer flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed border-border bg-card px-6 py-12 text-center transition-colors",
          "hover:border-primary/50 hover:bg-muted/50",
          "focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none",
          dragOver && "border-primary bg-primary/5",
        )}
      >
        <div className="flex size-12 items-center justify-center rounded-full bg-primary/10">
          <CloudUpload className="size-6 text-primary" aria-hidden />
        </div>
        <div>
          <p className="font-medium">
            {busy
              ? "Add more files to the queue"
              : "Drop files here, or click to browse"}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Multiple files OK. Split into 8 MB chunks automatically.
          </p>
        </div>
        <input
          ref={inputRef}
          type="file"
          multiple
          className="sr-only"
          aria-hidden
          tabIndex={-1}
          onChange={(e) => {
            addFiles(e.target.files);
            e.target.value = "";
          }}
        />
      </div>

      {state.uploading && (
        <Card className="group w-full overflow-visible">
          <CardContent className="flex min-w-0 flex-col gap-2">
            <div className="flex min-w-0 items-start gap-2">
              <div className="min-w-0 flex-1">
                {processing ? (
                  <div className="flex flex-col gap-2" role="status" aria-live="polite">
                    <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
                      <Loader2
                        className="size-4 shrink-0 animate-spin text-primary"
                        aria-hidden
                      />
                      <span className="min-w-0 flex-1 truncate">
                        Processing {state.uploading.fileName}
                      </span>
                      <span className="shrink-0 text-muted-foreground tabular-nums">
                        Finalizing…
                      </span>
                    </div>
                    <div className="relative h-1 w-full overflow-hidden rounded-full bg-muted">
                      <div className="absolute inset-y-0 w-2/5 rounded-full bg-primary animate-upload-indeterminate" />
                    </div>
                    {state.uploading.total > 0 && (
                      <p className="text-xs text-muted-foreground tabular-nums">
                        {formatBytes(state.uploading.total)} uploaded
                      </p>
                    )}
                  </div>
                ) : (
                  <>
                    <Progress value={state.uploading.percent}>
                      <div className="flex w-full min-w-0 items-center gap-2">
                        <ProgressLabel className="min-w-0 flex-1 truncate">
                          Uploading {state.uploading.fileName}
                        </ProgressLabel>
                        <ProgressValue />
                      </div>
                    </Progress>
                    <p className="mt-2 text-xs text-muted-foreground tabular-nums">
                      {formatBytes(state.uploading.sent)}
                      {state.uploading.total > 0
                        ? ` / ${formatBytes(state.uploading.total)}`
                        : ""}
                      {" · "}
                      {formatSpeed(state.uploading.bytesPerSec)}
                    </p>
                  </>
                )}
              </div>
              {state.canCancel && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="shrink-0"
                  aria-label={`Cancel upload of ${state.uploading.fileName}`}
                  onClick={() => cancelUpload()}
                >
                  <X aria-hidden />
                </Button>
              )}
            </div>
            <p className="text-xs text-muted-foreground">
              {processing
                ? "Assembling chunks on the server…"
                : "Continues in the background across routes and other open tabs."}
            </p>
          </CardContent>
        </Card>
      )}

      {state.queue.length > 0 && (
        <Card className="w-full">
          <CardContent className="flex flex-col gap-2">
            <p className="text-sm font-medium">
              Queue ({state.queue.length})
            </p>
            <ul className="flex flex-col gap-1.5">
              {state.queue.map((item) => (
                <li
                  key={item.id}
                  className="flex items-center gap-2 text-sm"
                >
                  <FileIcon
                    className="size-4 shrink-0 text-muted-foreground"
                    aria-hidden
                  />
                  <span className="min-w-0 flex-1 truncate">{item.name}</span>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {formatBytes(item.size)}
                  </span>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    aria-label={`Remove ${item.name} from queue`}
                    onClick={() => removeQueued(item.id)}
                  >
                    <X aria-hidden />
                  </Button>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      {state.results.length === 1 ? (
        <ResultCard
          result={state.results[0]}
          onDismiss={() => dismissResult(state.results[0].fileId)}
        />
      ) : state.results.length > 1 ? (
        <Card className="w-full overflow-visible">
          <CardContent className="flex flex-col gap-0 px-0 py-0">
            <div className="flex items-center justify-between gap-2 px-(--card-spacing) pt-(--card-spacing)">
              <p className="text-sm font-medium text-muted-foreground">
                {state.results.length} uploads
              </p>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="shrink-0"
                onClick={() => dismissAllResults()}
              >
                <RotateCcw aria-hidden /> Dismiss all
              </Button>
            </div>
            <Accordion
              key={state.results[0].fileId}
              defaultValue={[state.results[0].fileId]}
              className="px-(--card-spacing)"
            >
              {state.results.map((result) => (
                <AccordionItem key={result.fileId} value={result.fileId}>
                  <AccordionTrigger className="gap-2 hover:no-underline">
                    <span className="min-w-0 flex-1 truncate pr-2">
                      {result.fileName}
                    </span>
                    <span className="shrink-0 text-xs font-normal text-muted-foreground tabular-nums">
                      {formatBytes(result.fileSize)}
                    </span>
                  </AccordionTrigger>
                  <AccordionContent className="h-auto pb-4 [&_a]:no-underline [&_p:not(:last-child)]:mb-0">
                    <ResultBody
                      result={result}
                      onDismiss={() => dismissResult(result.fileId)}
                      compact
                    />
                  </AccordionContent>
                </AccordionItem>
              ))}
            </Accordion>
          </CardContent>
        </Card>
      ) : null}
    </div>
  );
}

function ResultCard({
  result,
  onDismiss,
}: {
  result: UploadResult;
  onDismiss: () => void;
}) {
  return (
    <Card className="w-full overflow-visible">
      <CardContent className="flex flex-col gap-4">
        <ResultBody result={result} onDismiss={onDismiss} />
      </CardContent>
    </Card>
  );
}

function ResultBody({
  result,
  onDismiss,
  compact = false,
}: {
  result: UploadResult;
  onDismiss: () => void;
  compact?: boolean;
}) {
  const [showToken, setShowToken] = useState(Boolean(result.accessToken));

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {!compact ? (
        <div className="flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10">
            <FileIcon className="size-5 text-primary" aria-hidden />
          </div>
          <div className="min-w-0 flex-1">
            <p className="truncate font-medium">{result.fileName}</p>
            <p className="text-sm text-muted-foreground">
              {formatBytes(result.fileSize)}
            </p>
            <div className="mt-1.5 flex flex-wrap items-center gap-1.5">
              <Badge
                variant={
                  result.status === "duplicate" ? "outline" : "secondary"
                }
              >
                {result.status ?? "ready"}
              </Badge>
              {result.visibility && (
                <Badge variant="secondary">{result.visibility}</Badge>
              )}
              {result.expiresAt && (
                <span className="text-xs text-muted-foreground">
                  Expires {formatDate(result.expiresAt)}
                </span>
              )}
            </div>
            <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
              {result.fileId}
            </p>
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0"
            onClick={onDismiss}
          >
            <RotateCcw aria-hidden /> Dismiss
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant={result.status === "duplicate" ? "outline" : "secondary"}
          >
            {result.status ?? "ready"}
          </Badge>
          {result.visibility && (
            <Badge variant="secondary">{result.visibility}</Badge>
          )}
          {result.expiresAt && (
            <span className="text-xs text-muted-foreground">
              Expires {formatDate(result.expiresAt)}
            </span>
          )}
          <p className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
            {result.fileId}
          </p>
          <Button
            variant="ghost"
            size="sm"
            className="shrink-0"
            onClick={onDismiss}
          >
            <RotateCcw aria-hidden /> Dismiss
          </Button>
        </div>
      )}

      {showToken && result.accessToken ? (
        <TokenRevealPanel
          reveals={[
            {
              fileId: result.fileId,
              fileName: result.fileName,
              accessToken: result.accessToken,
            },
          ]}
          onDismiss={() => setShowToken(false)}
        />
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="secondary"
          size="sm"
          onClick={async () => {
            try {
              if (typeof navigator.share === "function") {
                await navigator.share({
                  title: result.fileName,
                  url: result.longURL,
                });
                return;
              }
              await navigator.clipboard.writeText(result.longURL);
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
          href={result.longURL}
          target="_blank"
          rel="noreferrer"
          className={buttonVariants({ variant: "outline", size: "sm" })}
        >
          <ExternalLink aria-hidden /> Open
        </a>
        <a
          href={result.longDownloadURL}
          className={buttonVariants({ variant: "default", size: "sm" })}
        >
          <Download aria-hidden /> Download
        </a>
        <Link
          href={buildInspectPath(result.fileId, result.accessToken)}
          className={buttonVariants({ variant: "secondary", size: "sm" })}
        >
          <Search aria-hidden /> Inspect
        </Link>
      </div>

      <div className="grid min-w-0 gap-4 sm:grid-cols-[160px_1fr]">
        <ShareQR value={result.longURL} />
        <div className="flex min-w-0 flex-col gap-3">
          <LinkRow label="Share link" href={result.longURL} />
          <LinkRow label="Download" href={result.longDownloadURL} />
        </div>
      </div>
    </div>
  );
}

function LinkRow({ label, href }: { label: string; href: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex min-w-0 items-center gap-2">
        <Input
          readOnly
          value={href}
          className="min-w-0 font-mono text-xs"
          aria-label={label}
        />
        <CopyButton value={href} label={`Copy ${label.toLowerCase()}`} />
      </div>
    </div>
  );
}
