import type { Metadata } from "next";

import { MePanel } from "@/components/me-panel";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const metadata: Metadata = { title: "Account" };

export default function MePage() {
  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Account" }]} />
      <MePanel />
    </div>
  );
}
