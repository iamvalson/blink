"use client";

import { cn } from "@/lib/utils";
import type { ContentFormat } from "@/types/post";
import { Film, Tv2 } from "lucide-react";

interface FormatOption {
  id: ContentFormat;
  name: string;
  description: string;
  icon: React.ElementType;
}

const FORMAT_OPTIONS: FormatOption[] = [
  {
    id: "VIDEO",
    name: "Normal Video",
    description: "Publish a standard YouTube video (landscape or portrait).",
    icon: Tv2,
  },
  {
    id: "SHORT",
    name: "YouTube Short",
    description:
      "Publish a vertical or square short-form video (up to 3 minutes / 180 seconds).",
    icon: Film,
  },
];

interface YouTubeFormatSelectorProps {
  value: ContentFormat;
  onChange: (format: ContentFormat) => void;
  disabled?: boolean;
}

export function YouTubeFormatSelector({
  value,
  onChange,
  disabled,
}: YouTubeFormatSelectorProps) {
  return (
    <div className="space-y-2">
      <p className="text-sm font-medium text-foreground">YouTube video type</p>
      <p className="text-xs text-muted-foreground">
        Select whether this video is intended as a Normal Video or a YouTube Short.
      </p>
      <div className="grid gap-2 sm:grid-cols-2">
        {FORMAT_OPTIONS.map((option) => {
          const Icon = option.icon;
          const isSelected = value === option.id;

          return (
            <button
              key={option.id}
              type="button"
              disabled={disabled}
              onClick={() => !disabled && onChange(option.id)}
              className={cn(
                "relative flex flex-col items-start gap-2 rounded-lg border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                isSelected
                  ? "border-primary bg-primary/5"
                  : "border-border bg-background hover:bg-muted/50",
                disabled && "cursor-not-allowed opacity-50",
              )}
              aria-pressed={isSelected}
              aria-label={option.name}
            >
              <div className="flex w-full items-center justify-between">
                <div
                  className={cn(
                    "flex h-7 w-7 items-center justify-center rounded-md border",
                    isSelected
                      ? "border-primary bg-primary text-primary-foreground"
                      : "border-border bg-muted text-muted-foreground",
                  )}
                >
                  <Icon className="h-3.5 w-3.5" />
                </div>
              </div>
              <div>
                <p className="text-sm font-medium leading-none">
                  {option.name}
                </p>
                <p className="mt-1 text-xs text-muted-foreground leading-relaxed">
                  {option.description}
                </p>
              </div>
            </button>
          );
        })}
      </div>

      {value === "SHORT" && (
        <p className="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
          <strong>Note:</strong> YouTube classifies videos as Shorts if they are square or vertical (height ≥ width) and up to 3 minutes (180 seconds). Client-side validation checks duration and aspect ratio before uploading.
        </p>
      )}
    </div>
  );
}
