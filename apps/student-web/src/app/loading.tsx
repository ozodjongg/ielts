import { Skeleton } from "@/components/ui";

export default function Loading() {
  return (
    <div className="content" aria-busy="true" aria-label="Yuklanmoqda">
      <Skeleton className="h-8 w-64" />
      <Skeleton className="mt-3 h-4 w-96 max-w-full" />
      <div className="grid grid-3 section">
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
      </div>
    </div>
  );
}
