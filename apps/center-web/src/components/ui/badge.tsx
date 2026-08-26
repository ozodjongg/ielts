import type { HTMLAttributes, ReactNode } from "react";
import { cn } from "@/lib/utils";

export function Badge({ className, ...props }: HTMLAttributes<HTMLSpanElement>) {
  return <span data-slot="badge" className={cn("inline-flex items-center rounded-full border border-[var(--border)] bg-[var(--muted-background)] px-2.5 py-0.5 text-xs font-medium text-[var(--muted)]", className)} {...props} />;
}
export function Pill({ children, tone = "", className }: { children: ReactNode; tone?: "ok" | "bad" | ""; className?: string }) {
  return <Badge className={cn(tone === "ok" ? "border-[var(--foreground)] text-[var(--foreground)]" : tone === "bad" ? "border-[var(--border)] text-[var(--muted)]" : "", className)}>{children}</Badge>;
}
