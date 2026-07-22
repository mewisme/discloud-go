import type { Metadata } from "next";

import { FileInspectPanel } from "@/components/file-inspect";

type Props = {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ token?: string | string[] }>;
};

function firstParam(
  value: string | string[] | undefined,
): string | undefined {
  if (Array.isArray(value)) return value[0];
  return value || undefined;
}

export async function generateMetadata({
  params,
  searchParams,
}: Props): Promise<Metadata> {
  const { id } = await params;
  const token = firstParam((await searchParams).token);
  // Never put raw token in title / description / OG / canonical.
  return {
    title: `Inspect ${id.slice(0, 8)}…`,
    ...(token ? { referrer: "no-referrer" as const } : {}),
  };
}

export default async function InspectPage({ params, searchParams }: Props) {
  const { id } = await params;
  const token = firstParam((await searchParams).token);
  return <FileInspectPanel fileId={id} accessToken={token} />;
}
