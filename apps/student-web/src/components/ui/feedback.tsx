import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Empty({ children = "Hozircha ma’lumot yo‘q." }: { children?: ReactNode }) {
  return <div className="rounded-xl border border-dashed border-[var(--border)] bg-[var(--card)] p-8 text-center text-sm text-[var(--muted)]">{children}</div>;
}
export function Loading({ label = "Yuklanmoqda…" }: { label?: string }) {
  return <div role="status" aria-live="polite" className="text-sm text-[var(--muted)]">{label}</div>;
}
export function Alert({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div role="alert" className={cn("rounded-lg border border-[var(--border)] bg-[var(--muted-background)] p-3 text-sm text-[var(--foreground)]", className)} {...props} />;
}
export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div aria-hidden="true" className={cn("animate-pulse rounded-md bg-[var(--muted-background)]", className)} {...props} />;
}
export function Progress({ value, className }: { value: number; className?: string }) {
  const safe = Math.min(100, Math.max(0, Number.isFinite(value) ? value : 0));
  return <div role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={safe} className={cn("h-2 overflow-hidden rounded-full bg-[var(--muted-background)]", className)}><div className="h-full bg-[var(--foreground)] transition-[width]" style={{ width: `${safe}%` }} /></div>;
}
