import type { Metadata } from "next";
import { Download, FolderOpen, TriangleAlert } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { buttonVariants } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { getFiles } from "@/lib/api";
import { formatBytes, formatDate } from "@/lib/format";

export const metadata: Metadata = { title: "Files" };
export const dynamic = "force-dynamic";

export default async function FilesPage() {
  let files;
  try {
    files = await getFiles();
  } catch {
    return (
      <EmptyState
        icon={<TriangleAlert className="size-6 text-amber-500" aria-hidden />}
        title="Could not load files"
        text="The API is unreachable right now. Check that the backend is running, then refresh."
      />
    );
  }

  if (files.length === 0) {
    return (
      <EmptyState
        icon={<FolderOpen className="size-6 text-muted-foreground" aria-hidden />}
        title="No files yet"
        text="Files you upload will show up here with their share and download links."
      />
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-baseline justify-between">
        <h1 className="text-2xl font-semibold tracking-tight">Recent files</h1>
        <Badge variant="secondary">{files.length} file{files.length === 1 ? "" : "s"}</Badge>
      </div>
      <div className="overflow-hidden rounded-xl border border-border/60">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              <TableHead className="w-28">Size</TableHead>
              <TableHead className="w-44">Uploaded</TableHead>
              <TableHead className="w-16 text-right">
                <span className="sr-only">Actions</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {files.map((f) => (
              <TableRow key={f.fileId}>
                <TableCell className="max-w-0 truncate font-medium">
                  <a
                    href={`/f/${f.fileId}/${f.fileName}`}
                    className="hover:underline"
                    target="_blank"
                    rel="noreferrer"
                  >
                    {f.fileName}
                  </a>
                </TableCell>
                <TableCell className="tabular-nums text-muted-foreground">
                  {formatBytes(f.fileSize)}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatDate(f.createdAt)}
                </TableCell>
                <TableCell className="text-right">
                  <a
                    href={`/f/${f.fileId}/${f.fileName}?download=1`}
                    className={buttonVariants({ variant: "ghost", size: "icon-sm" })}
                    aria-label={`Download ${f.fileName}`}
                  >
                    <Download aria-hidden />
                  </a>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}

function EmptyState({
  icon,
  title,
  text,
}: {
  icon: React.ReactNode;
  title: string;
  text: string;
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
      {icon}
      <h1 className="font-semibold">{title}</h1>
      <p className="max-w-sm text-sm text-muted-foreground">{text}</p>
    </div>
  );
}
