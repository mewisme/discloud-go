import type { Metadata } from "next";

import { MeTokensPanel } from "@/components/me-panel";

export const metadata: Metadata = { title: "API tokens" };

export default function MeTokensPage() {
  return <MeTokensPanel />;
}
