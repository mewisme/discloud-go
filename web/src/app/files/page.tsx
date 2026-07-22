import type { Metadata } from "next";

import { FilesList } from "@/components/files-list";

export const metadata: Metadata = { title: "My files" };

export default function FilesPage() {
  return <FilesList />;
}
