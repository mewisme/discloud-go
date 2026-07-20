import { Skeleton } from "@/components/ui/skeleton";

export default function FilesLoading() {
  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-baseline justify-between">
        <Skeleton className="h-8 w-40" />
        <Skeleton className="h-5 w-16" />
      </div>
      <div className="flex flex-col gap-3 rounded-xl border border-border/60 p-4">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="flex items-center gap-4">
            <Skeleton className="h-5 flex-1" />
            <Skeleton className="h-5 w-24" />
            <Skeleton className="h-5 w-40" />
            <Skeleton className="size-7" />
          </div>
        ))}
      </div>
    </div>
  );
}
