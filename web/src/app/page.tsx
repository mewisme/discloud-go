import { Infinity as InfinityIcon, Shield, Zap } from "lucide-react";

import { Uploader } from "@/components/uploader";

const features = [
  {
    icon: InfinityIcon,
    title: "Unlimited storage",
    text: "Files are split into 8 MB chunks and stored as Discord attachments — no size ceiling.",
  },
  {
    icon: Zap,
    title: "Fast transfers",
    text: "Chunks upload concurrently and downloads stream straight from the CDN with range support.",
  },
  {
    icon: Shield,
    title: "Fresh links",
    text: "Signed CDN URLs are refreshed automatically, so share links never go stale.",
  },
];

export default function Home() {
  return (
    <div className="flex flex-col gap-12">
      <section className="mx-auto flex max-w-xl flex-col items-center gap-3 pt-4 text-center">
        <h1 className="text-4xl font-semibold tracking-tight">
          Unlimited cloud storage
        </h1>
        <p className="text-balance text-muted-foreground">
          Upload any file and get a shareable link. Your data lives as chunked
          attachments on Discord — free, durable, and fast.
        </p>
      </section>

      <section className="mx-auto w-full max-w-xl">
        <Uploader />
      </section>

      <section className="grid gap-4 sm:grid-cols-3">
        {features.map(({ icon: Icon, title, text }) => (
          <div
            key={title}
            className="rounded-xl border border-border/60 bg-card p-5"
          >
            <Icon className="size-5 text-primary" aria-hidden />
            <h2 className="mt-3 text-sm font-semibold">{title}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{text}</p>
          </div>
        ))}
      </section>
    </div>
  );
}
