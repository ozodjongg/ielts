"use client";

import type { ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export type ButtonVariant = "default" | "outline" | "ghost" | "destructive";

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: "sm" | "default" | "lg" | "icon";
}

const sizes = {
  sm: "h-8 px-3 text-xs",
  default: "h-9 px-4 text-sm",
  lg: "h-10 px-6 text-sm",
  icon: "size-9 p-0",
};

export function Button({ className = "", variant, size = "default", type = "button", ...props }: ButtonProps) {
  const legacyPrimary = /(^|\s)(primary|accent)(\s|$)/.test(className);
  const legacyDanger = /(^|\s)danger(\s|$)/.test(className);
  const clean = className.replace(/(^|\s)(primary|accent|danger)(?=\s|$)/g, " ").trim();
  const resolved = variant ?? (legacyDanger ? "destructive" : legacyPrimary ? "default" : "outline");
  const variants: Record<ButtonVariant, string> = {
    default: "border-[var(--accent)] bg-[var(--accent)] text-[var(--accent-foreground)] hover:opacity-85",
    outline: "border-[var(--border)] bg-[var(--card)] text-[var(--foreground)] hover:bg-[var(--muted-background)]",
    ghost: "border-transparent bg-transparent text-[var(--foreground)] hover:bg-[var(--muted-background)]",
    destructive: "border-[var(--foreground)] bg-[var(--card)] text-[var(--foreground)] hover:bg-[var(--muted-background)]",
  };
  return (
    <button
      type={type}
      data-slot="button"
      className={cn(
        "inline-flex shrink-0 items-center justify-center gap-2 rounded-md border font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
        sizes[size],
        variants[resolved],
        clean,
      )}
      {...props}
    />
  );
}
