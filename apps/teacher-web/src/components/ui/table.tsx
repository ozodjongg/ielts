import type { HTMLAttributes, TableHTMLAttributes, ThHTMLAttributes, TdHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export function TableWrap({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("relative w-full overflow-auto rounded-lg border border-[var(--border)]", className)} {...props} />;
}
export function Table({ className, ...props }: TableHTMLAttributes<HTMLTableElement>) {
  return <table data-slot="table" className={cn("w-full caption-bottom text-sm", className)} {...props} />;
}
export function TableHeader({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <thead data-slot="table-header" className={cn("[&_tr]:border-b", className)} {...props} />;
}
export function TableBody({ className, ...props }: HTMLAttributes<HTMLTableSectionElement>) {
  return <tbody data-slot="table-body" className={cn("[&_tr:last-child]:border-0", className)} {...props} />;
}
export function TableRow({ className, ...props }: HTMLAttributes<HTMLTableRowElement>) {
  return <tr data-slot="table-row" className={cn("border-b border-[var(--border)] transition-colors hover:bg-[var(--muted-background)]", className)} {...props} />;
}
export function TableHead({ className, ...props }: ThHTMLAttributes<HTMLTableCellElement>) {
  return <th data-slot="table-head" className={cn("h-10 px-3 text-left align-middle text-xs font-medium uppercase tracking-wide text-[var(--muted)]", className)} {...props} />;
}
export function TableCell({ className, ...props }: TdHTMLAttributes<HTMLTableCellElement>) {
  return <td data-slot="table-cell" className={cn("p-3 align-middle", className)} {...props} />;
}
