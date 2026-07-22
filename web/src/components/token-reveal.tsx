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

/** One-time private token reveal — component state only; dismiss loses the raw token. */
export function TokenRevealPanel({
  reveal,
  onDismiss,
}: {
  reveal: TokenReveal;
  onDismiss: () => void;
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
    <Card className="w-full border-amber-500/40 bg-amber-500/5">
      <CardContent className="relative flex flex-col gap-3 pr-10">
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
          <p className="font-medium">Private access token</p>
          <p className="mt-1 text-sm text-muted-foreground">
            Shown once. Copy it now — closing this panel clears it from the
            page. Rotate later if you lose it. Switching to public invalidates
            the token.
          </p>
        </div>
        <Field label="Token" value={accessToken} />
        <Field label="Private view URL" value={viewURL} />
        <Field label="Private download URL" value={downloadURL} />
        <Field label="Private inspect URL" value={inspectURL} />
      </CardContent>
    </Card>
  );
}

function Field({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-xs font-medium text-muted-foreground">{label}</span>
      <div className="flex items-center gap-2">
        <Input
          readOnly
          value={value}
          className="font-mono text-xs"
          aria-label={label}
        />
        <CopyButton value={value} label={`Copy ${label.toLowerCase()}`} />
      </div>
    </div>
  );
}
