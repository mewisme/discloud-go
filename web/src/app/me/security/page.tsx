import type { Metadata } from "next";

import { MeSecurityPanel } from "@/components/me-panel";

export const metadata: Metadata = { title: "Security" };

export default function MeSecurityPage() {
  return <MeSecurityPanel />;
}
