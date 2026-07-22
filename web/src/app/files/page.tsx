import type { Metadata } from "next";

import { FilesList } from "@/components/files-list";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const metadata: Metadata = { title: "My files" };

export default function FilesPage() {
  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Files" }]} />
      <FilesList />
    </div>
  );
}
