"use client";

import { cn } from "@/lib/utils";
import type { ContentFormat } from "@/types/post";
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  Film,
  Loader2,
  RefreshCcw,
  Tv2,
} from "lucide-react";

interface StatusBadgeProps {
  status: string;
  className?: string;
}

const STATUS_CONFIG: Record<
  string,
  { label: string; colorClass: string; icon: React.ElementType | null; spin?: boolean }
> = {
  QUEUED: {
    label: "Queued",
    colorClass:
      "bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-800",
    icon: Clock,
  },
  PENDING: {
    label: "Pending",
    colorClass:
      "bg-blue-50 text-blue-700 border-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-800",
    icon: Clock,
  },
  PROCESSING: {
    label: "Processing",
    colorClass:
      "bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-800",
    icon: Loader2,
    spin: true,
  },
  PUBLISHING: {
    label: "Publishing",
    colorClass:
      "bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-800",
    icon: Loader2,
    spin: true,
  },
  PUBLISHED: {
    label: "Published",
    colorClass:
      "bg-emerald-50 text-emerald-700 border-emerald-200 dark:bg-emerald-950 dark:text-emerald-300 dark:border-emerald-800",
    icon: CheckCircle2,
  },
  FAILED: {
    label: "Failed",
    colorClass:
      "bg-red-50 text-red-700 border-red-200 dark:bg-red-950 dark:text-red-300 dark:border-red-800",
    icon: AlertCircle,
  },
  UNKNOWN: {
    label: "Unknown",
    colorClass:
      "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400",
    icon: RefreshCcw,
  },
  RETRYING: {
    label: "Retrying",
    colorClass:
      "bg-amber-50 text-amber-700 border-amber-200 dark:bg-amber-950 dark:text-amber-300 dark:border-amber-800",
    icon: RefreshCcw,
    spin: true,
  },
  CANCELLED: {
    label: "Cancelled",
    colorClass:
      "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400",
    icon: null,
  },
};

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const cfg = STATUS_CONFIG[status] ?? {
    label: status,
    colorClass:
      "bg-neutral-100 text-neutral-600 border-neutral-200",
    icon: null,
  };
  const Icon = cfg.icon;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        cfg.colorClass,
        className,
      )}
    >
      {Icon && (
        <Icon
          className={cn("h-3 w-3 shrink-0", cfg.spin && "animate-spin")}
          aria-hidden
        />
      )}
      {cfg.label}
    </span>
  );
}

interface FormatBadgeProps {
  format: ContentFormat;
  className?: string;
}

const FORMAT_CONFIG: Record<
  ContentFormat,
  { label: string; icon: React.ElementType }
> = {
  VIDEO: { label: "Video", icon: Tv2 },
  SHORT: { label: "Short", icon: Film },
};

export function FormatBadge({ format, className }: FormatBadgeProps) {
  const cfg = FORMAT_CONFIG[format] ?? { label: format, icon: Tv2 };
  const Icon = cfg.icon;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-md border border-border bg-muted/50 px-1.5 py-0.5 text-xs text-muted-foreground",
        className,
      )}
    >
      <Icon className="h-3 w-3 shrink-0" aria-hidden />
      {cfg.label}
    </span>
  );
}
