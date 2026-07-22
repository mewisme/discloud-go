import type { Metadata } from "next";

import { FileInspectPanel } from "@/components/file-inspect";

type Props = { params: Promise<{ id: string }> };

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { id } = await params;
  return { title: `Inspect ${id.slice(0, 8)}…` };
}

export default async function InspectPage({ params }: Props) {
  const { id } = await params;
  return <FileInspectPanel fileId={id} />;
}
