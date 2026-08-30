"use client";

import type {
  InputHTMLAttributes,
  LabelHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from "react";

import { cn } from "@/lib/utils";

const control =
  "w-full rounded-md border border-[var(--border)] bg-[var(--card)] px-3 text-sm text-[var(--foreground)] shadow-sm outline-none transition placeholder:text-[var(--muted)] focus:border-[var(--accent)] focus:ring-2 focus:ring-[var(--accent)]/15 disabled:cursor-not-allowed disabled:opacity-50 aria-[invalid=true]:border-[var(--foreground)] aria-[invalid=true]:ring-2 aria-[invalid=true]:ring-[var(--ring)]/10";

export function Input({ className, ...props }: InputHTMLAttributes<HTMLInputElement>) {
  return <input data-slot="input" className={cn(control, "h-9", className)} {...props} />;
}

export function Select({ className, children, ...props }: SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <select data-slot="select" className={cn(control, "h-9 pr-8", className)} {...props}>
      {children}
    </select>
  );
}

export function Textarea({ className, ...props }: TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return <textarea data-slot="textarea" className={cn(control, "min-h-28 py-2", className)} {...props} />;
}

export function Label({ className, ...props }: LabelHTMLAttributes<HTMLLabelElement>) {
  return <label data-slot="label" className={cn("text-xs font-medium text-[var(--foreground)]", className)} {...props} />;
}

export function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: ReactNode }) {
  return (
    <label data-slot="field" className="grid gap-1.5 text-xs font-medium text-[var(--foreground)]">
      <span>{label}</span>
      {children}
      {hint ? <span className="font-normal text-[var(--muted)]">{hint}</span> : null}
    </label>
  );
}

export function Switch({
  checked,
  onCheckedChange,
  disabled,
  "aria-label": ariaLabel,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  "aria-label"?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onCheckedChange(!checked)}
      className={cn(
        "relative inline-flex h-6 w-11 items-center rounded-full border transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "border-[var(--foreground)] bg-[var(--foreground)]" : "border-[var(--border)] bg-[var(--muted-background)]",
      )}
    >
      <span
        aria-hidden="true"
        className={cn(
          "inline-block size-4 rounded-full bg-[var(--background)] shadow transition-transform",
          checked ? "translate-x-6" : "translate-x-1",
        )}
      />
    </button>
  );
}
