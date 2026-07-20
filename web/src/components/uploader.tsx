"use client";

import { CloudUpload, FileIcon, RotateCcw } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import { toast } from "sonner";

import { CopyButton } from "@/components/copy-button";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Progress,
  ProgressLabel,
  ProgressValue,
} from "@/components/ui/progress";
import type { UploadResult } from "@/lib/api";
import { uploadFileChunked } from "@/lib/chunked-upload";
import { formatBytes } from "@/lib/format";
import { rememberLocalFile } from "@/lib/local-files";
import { cn } from "@/lib/utils";

type UploadState =
  | { phase: "idle" }
  | { phase: "uploading"; fileName: string; percent: number }
  | { phase: "done"; result: UploadResult };

export function Uploader() {
  const [state, setState] = useState<UploadState>({ phase: "idle" });
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const upload = useCallback((file: File) => {
    setState({ phase: "uploading", fileName: file.name, percent: 0 });
    uploadFileChunked(file, (sent, total) => {
      setState({
        phase: "uploading",
        fileName: file.name,
        percent: Math.round((sent / total) * 100),
      });
    })
      .then((result: UploadResult) => {
        rememberLocalFile(result);
        window.dispatchEvent(new Event("discloud:files"));
        setState({ phase: "done", result });
        toast.success(`${result.fileName} uploaded`);
      })
      .catch((err: unknown) => {
        setState({ phase: "idle" });
        toast.error(err instanceof Error ? err.message : "Upload failed", {
          description: "Retrying the upload will skip chunks that already made it through.",
        });
      });
  }, []);

  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setDragOver(false);
      const file = e.dataTransfer.files[0];
      if (file) upload(file);
    },
    [upload],
  );

  if (state.phase === "done") {
    const { result } = state;
    return (
      <Card className="w-full">
        <CardContent className="flex flex-col gap-4">
          <div className="flex items-center gap-3">
            <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10">
              <FileIcon className="size-5 text-primary" aria-hidden />
            </div>
            <div className="min-w-0">
              <p className="truncate font-medium">{result.fileName}</p>
              <p className="text-sm text-muted-foreground">
                {formatBytes(result.fileSize)}
              </p>
            </div>
            <Button
              variant="ghost"
              size="sm"
              className="ml-auto"
              onClick={() => setState({ phase: "idle" })}
            >
              <RotateCcw aria-hidden /> Upload another
            </Button>
          </div>
          <LinkRow label="Share link" href={result.longURL} />
          <LinkRow label="Direct download" href={result.longDownloadURL} />
        </CardContent>
      </Card>
    );
  }

  if (state.phase === "uploading") {
    return (
      <Card className="w-full">
        <CardContent>
          <Progress value={state.percent} className="gap-2">
            <ProgressLabel className="truncate">
              Uploading {state.fileName}
            </ProgressLabel>
            <ProgressValue />
          </Progress>
        </CardContent>
      </Card>
    );
  }

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label="Upload a file"
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
        <p className="font-medium">Drop a file here, or click to browse</p>
        <p className="mt-1 text-sm text-muted-foreground">
          Any file type. Stored in 8 MB chunks on Discord.
        </p>
      </div>
      <input
        ref={inputRef}
        type="file"
        className="sr-only"
        aria-hidden
        tabIndex={-1}
        onChange={(e) => {
          const file = e.target.files?.[0];
          if (file) upload(file);
          e.target.value = "";
        }}
      />
    </div>
  );
}

function LinkRow({ label, href }: { label: string; href: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex items-center gap-2">
        <Input readOnly value={href} className="font-mono text-xs" aria-label={label} />
        <CopyButton value={href} label={`Copy ${label.toLowerCase()}`} />
      </div>
    </div>
  );
}
