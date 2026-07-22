"use client";

import { X } from "lucide-react";

import { CopyButton } from "@/components/copy-button";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { buildFileURL, buildInspectPath } from "@/lib/api";

export type TokenReveal = {
  fileId: string;
  fileName: string;
  accessToken: string;
};

/** One-time private token reveal — component state only; dismiss loses the raw tokens. */
export function TokenRevealPanel({
  reveals,
  onDismiss,
}: {
  reveals: TokenReveal[];
  onDismiss: () => void;
}) {
  if (reveals.length === 0) return null;

  return (
    <Card className="w-full border-geist-amber/40 bg-geist-amber-soft">
      <CardContent className="relative flex flex-col gap-4 pr-10">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="absolute top-0 right-0"
          aria-label="Dismiss token reveal"
          onClick={onDismiss}
        >
          <X aria-hidden />
        </Button>
        <div>
          <p className="font-medium">
            Private access token{reveals.length > 1 ? "s" : ""}
          </p>
          <p className="mt-1 text-sm text-muted-foreground">
            Shown once. Copy now — closing this panel clears them from the page.
            Rotate later if you lose a token. Switching to public invalidates it.
          </p>
        </div>
        {reveals.map((reveal) => (
          <RevealFields key={reveal.fileId} reveal={reveal} showName={reveals.length > 1} />
        ))}
      </CardContent>
    </Card>
  );
}

function RevealFields({
  reveal,
  showName,
}: {
  reveal: TokenReveal;
  showName: boolean;
}) {
  const { fileId, fileName, accessToken } = reveal;
  const viewURL = buildFileURL({ fileId, fileName, token: accessToken });
  const downloadURL = buildFileURL({
    fileId,
    fileName,
    download: true,
    token: accessToken,
  });
  const inspectURL =
    typeof window !== "undefined"
      ? `${window.location.origin}${buildInspectPath(fileId, accessToken)}`
      : buildInspectPath(fileId, accessToken);

  return (
    <div className="flex flex-col gap-3 border-t border-border/60 pt-3 first:border-t-0 first:pt-0">
      {showName && <p className="truncate text-sm font-medium">{fileName}</p>}
      <Field label="Token" value={accessToken} />
      <Field label="Private view URL" value={viewURL} />
      <Field label="Private download URL" value={downloadURL} />
      <Field label="Private inspect URL" value={inspectURL} />
    </div>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex min-w-0 items-center gap-2">
        <Input
          readOnly
          value={value}
          className="min-w-0 font-mono text-xs"
          aria-label={label}
        />
        <CopyButton value={value} label={`Copy ${label.toLowerCase()}`} />
      </div>
    </div>
  );
}
