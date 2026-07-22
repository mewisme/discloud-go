"use client";

import { Check, Copy, X } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
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
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="absolute top-0 right-0"
                aria-label="Dismiss token reveal"
                onClick={onDismiss}
              />
            }
          >
            <X aria-hidden />
          </TooltipTrigger>
          <TooltipContent>Dismiss</TooltipContent>
        </Tooltip>
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
          <RevealFields
            key={reveal.fileId}
            reveal={reveal}
            showName={reveals.length > 1}
          />
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
    <FieldGroup className="gap-3 border-t border-border/60 pt-3 first:border-t-0 first:pt-0">
      {showName && <p className="truncate text-sm font-medium">{fileName}</p>}
      <CopyField label="Token" value={accessToken} />
      <CopyField label="Private view URL" value={viewURL} />
      <CopyField label="Private download URL" value={downloadURL} />
      <CopyField label="Private inspect URL" value={inspectURL} />
    </FieldGroup>
  );
}

function CopyField({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  const id = `copy-${label.replace(/\s+/g, "-").toLowerCase()}`;

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      toast.success("Link copied to clipboard");
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Could not copy to clipboard");
    }
  }

  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <InputGroup>
        <InputGroupInput
          id={id}
          readOnly
          value={value}
          className="min-w-0 font-mono text-xs"
          aria-label={label}
        />
        <InputGroupAddon align="inline-end">
          <Tooltip>
            <TooltipTrigger
              render={
                <InputGroupButton
                  size="icon-xs"
                  aria-label={`Copy ${label.toLowerCase()}`}
                  onClick={copy}
                />
              }
            >
              {copied ? (
                <Check className="text-geist-green" aria-hidden />
              ) : (
                <Copy aria-hidden />
              )}
            </TooltipTrigger>
            <TooltipContent>Copy</TooltipContent>
          </Tooltip>
        </InputGroupAddon>
      </InputGroup>
    </Field>
  );
}
