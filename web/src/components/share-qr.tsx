"use client";

import QRCode from "qrcode";
import { useEffect, useState } from "react";

import { cn } from "@/lib/utils";

export function ShareQR({
  value,
  className,
}: {
  value: string;
  className?: string;
}) {
  const [dataURL, setDataURL] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    QRCode.toDataURL(value, {
      margin: 1,
      width: 160,
      color: { dark: "#000000", light: "#ffffff" },
    })
      .then((url) => {
        if (!cancelled) setDataURL(url);
      })
      .catch(() => {
        if (!cancelled) setDataURL(null);
      });
    return () => {
      cancelled = true;
    };
  }, [value]);

  if (!dataURL) {
    return (
      <div
        className={cn(
          "flex size-40 items-center justify-center rounded-lg border border-border bg-muted text-xs text-muted-foreground",
          className,
        )}
      >
        QR…
      </div>
    );
  }

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={dataURL}
      alt="QR code for share link"
      width={160}
      height={160}
      className={cn("rounded-lg border border-border bg-white p-1", className)}
    />
  );
}
