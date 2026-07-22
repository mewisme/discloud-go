import type { Metadata } from "next";

import { MePanel } from "@/components/me-panel";

export const metadata: Metadata = { title: "Account" };

export default function MePage() {
  return <MePanel />;
}
