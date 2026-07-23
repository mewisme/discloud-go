import type { Metadata } from "next";

import { MePreferencesPanel } from "@/components/me-panel";

export const metadata: Metadata = { title: "Preferences" };

export default function MePreferencesPage() {
  return <MePreferencesPanel />;
}
