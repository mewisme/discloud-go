"use client";

import { Download, ExternalLink, FolderOpen, Trash2 } from "lucide-react";
import Link from "next/link";
import { useSyncExternalStore } from "react";

import { Badge } from "@/components/ui/badge";
import { Button, buttonVariants } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { apiURL } from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";
import {
  getLocalFilesServerSnapshot,
  getLocalFilesSnapshot,
  removeLocalFile,
  subscribeLocalFiles,
} from "@/lib/local-files";

export function FilesList() {
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
        the files on the server. Click a name to inspect.
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
                <TableRow key={f.fileId}>
                  <TableCell className="max-w-0 truncate font-medium">
                    <Link
                      href={`/i/${f.fileId}`}
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
                        onClick={() => removeLocalFile(f.fileId)}
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
    </div>
  );
}
