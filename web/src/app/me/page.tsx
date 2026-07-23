import type { Metadata } from "next";

import { MeOverviewPanel } from "@/components/me-panel";

export const metadata: Metadata = { title: "Account" };

export default function MePage() {
  return <MeOverviewPanel />;
}
