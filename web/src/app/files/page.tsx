import type { Metadata } from "next";

import { FilesList } from "@/components/files-list";

export const metadata: Metadata = { title: "Files" };

export default function FilesPage() {
  return <FilesList />;
}
