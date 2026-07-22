import type { Metadata } from "next";

import { FileInspectPanel } from "@/components/file-inspect";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

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
  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb
        items={[
          { label: "Files", href: "/files" },
          { label: `Inspect ${id.slice(0, 8)}…` },
        ]}
      />
      <FileInspectPanel fileId={id} accessToken={token} />
    </div>
  );
}
