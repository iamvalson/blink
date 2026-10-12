"use client";

import { AlertCircle, AlertTriangle, ArrowRight, CheckCircle2, Clock, Eye, Film, Loader2, Maximize2, Tv2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { ContentFormat } from "@/types/post";
import type { VideoValidationResult } from "@/features/posts/utils/video-validator";

interface VideoValidationCardProps {
  format: ContentFormat;
  validation: VideoValidationResult | null;
  isValidating?: boolean;
  onSwitchToNormalVideo?: () => void;
  fileName?: string;
}

export function VideoValidationCard({
  format,
  validation,
  isValidating,
  onSwitchToNormalVideo,
  fileName,
}: VideoValidationCardProps) {
  if (isValidating) {
    return (
      <div className="flex items-center gap-3 rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground animate-pulse">
        <Loader2 className="h-4 w-4 animate-spin text-primary shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="font-medium text-foreground">Analyzing video file…</p>
          <p className="text-[11px] truncate">
            {fileName ? `Reading metadata for ${fileName}` : "Extracting duration and aspect ratio locally in browser"}
          </p>
        </div>
      </div>
    );
  }

  if (!validation) {
    return null;
  }

  const { status, metadata, error, warning, suggestion, canSwitchToNormalVideo } = validation;
  const isShort = format === "SHORT";
  const formatLabel = isShort ? "YouTube Short" : "Normal Video";

  // Error state: corrupted / unreadable video
  if (status === "error" || error) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-xs space-y-2">
        <div className="flex items-start gap-2 text-destructive">
          <AlertCircle className="h-4 w-4 shrink-0 mt-0.5" />
          <div className="min-w-0 flex-1">
            <p className="font-semibold text-sm">Video Metadata Error</p>
            <p className="mt-1 leading-relaxed text-destructive/90">
              {error || "Unable to read video dimensions and duration. The file may be corrupted or unsupported."}
            </p>
            <p className="mt-1 text-[11px] text-destructive/80">
              Please choose a different, valid MP4/WebM video file before publishing.
            </p>
          </div>
        </div>
      </div>
    );
  }

  if (!metadata) {
    return null;
  }

  return (
    <div className="rounded-lg border border-border bg-card p-4 space-y-3 shadow-xs">
      {/* Header with selected type */}
      <div className="flex flex-wrap items-center justify-between gap-2 border-b pb-2.5">
        <div className="flex items-center gap-2">
          {isShort ? (
            <Film className="h-4 w-4 text-primary shrink-0" />
          ) : (
            <Tv2 className="h-4 w-4 text-primary shrink-0" />
          )}
          <span className="text-xs font-semibold text-foreground">
            Selected type: <span className="text-primary">{formatLabel}</span>
          </span>
        </div>

        {/* Status pill */}
        {status === "valid" ? (
          <span className="inline-flex items-center gap-1 rounded-full bg-emerald-50 px-2 py-0.5 text-[11px] font-medium text-emerald-700 dark:bg-emerald-950 dark:text-emerald-300 border border-emerald-200 dark:border-emerald-800">
            <CheckCircle2 className="h-3 w-3" />
            {isShort ? "Suitable for Shorts" : "Standard Video"}
          </span>
        ) : (
          <span className="inline-flex items-center gap-1 rounded-full bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-800 dark:bg-amber-950 dark:text-amber-300 border border-amber-200 dark:border-amber-800">
            <AlertTriangle className="h-3 w-3" />
            Shorts Eligibility Notice
          </span>
        )}
      </div>

      {/* Extracted metadata grid */}
      <div className="grid grid-cols-3 gap-2 text-xs">
        <div className="rounded-md bg-muted/40 p-2 border border-border/50">
          <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
            <Clock className="h-3 w-3" />
            <span>Duration</span>
          </div>
          <p className="mt-1 font-mono font-semibold text-foreground text-sm">
            {metadata.formattedDuration}
          </p>
          <p className="text-[10px] text-muted-foreground mt-0.5">
            {Math.round(metadata.duration)}s total
          </p>
        </div>

        <div className="rounded-md bg-muted/40 p-2 border border-border/50">
          <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
            <Maximize2 className="h-3 w-3" />
            <span>Resolution</span>
          </div>
          <p className="mt-1 font-mono font-semibold text-foreground text-sm">
            {metadata.resolution}
          </p>
          <p className="text-[10px] text-muted-foreground mt-0.5">
            {metadata.width} × {metadata.height}px
          </p>
        </div>

        <div className="rounded-md bg-muted/40 p-2 border border-border/50">
          <div className="flex items-center gap-1 text-[11px] text-muted-foreground">
            <Eye className="h-3 w-3" />
            <span>Orientation</span>
          </div>
          <p className="mt-1 font-semibold text-foreground text-sm">
            {metadata.orientation}
          </p>
          <p className="text-[10px] text-muted-foreground mt-0.5">
            ratio {metadata.aspectRatio.toFixed(2)}
          </p>
        </div>
      </div>

      {/* Status banner and actionable advice */}
      {status === "valid" ? (
        <div className="rounded-md bg-emerald-50/70 p-2.5 text-xs text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300 border border-emerald-200/60 dark:border-emerald-800/60 flex items-start gap-2">
          <CheckCircle2 className="h-4 w-4 shrink-0 mt-0.5 text-emerald-600 dark:text-emerald-400" />
          <div>
            <p className="font-medium">
              {isShort
                ? "Suitable for Shorts based on the basic checks."
                : "Suitable for standard video publishing."}
            </p>
            <p className="mt-0.5 text-[11px] text-emerald-700/90 dark:text-emerald-400/90">
              {isShort
                ? `Video is ${metadata.orientation.toLowerCase()} and under 3 minutes (180s). YouTube will classify it based on its automated indexing.`
                : "Standard videos support landscape and portrait orientations without duration restrictions."}
            </p>
          </div>
        </div>
      ) : (
        <div className="rounded-md bg-amber-50 p-3 text-xs text-amber-900 dark:bg-amber-950/60 dark:text-amber-200 border border-amber-200 dark:border-amber-800 space-y-2">
          <div className="flex items-start gap-2">
            <AlertTriangle className="h-4 w-4 shrink-0 mt-0.5 text-amber-600 dark:text-amber-400" />
            <div className="space-y-1">
              <p className="font-semibold text-amber-900 dark:text-amber-100">
                Shorts eligibility warning
              </p>
              <p className="leading-relaxed text-amber-800 dark:text-amber-300">
                {warning}
              </p>
              {suggestion && (
                <p className="text-[11px] text-amber-700 dark:text-amber-400">
                  {suggestion}
                </p>
              )}
            </div>
          </div>

          {canSwitchToNormalVideo && onSwitchToNormalVideo && (
            <div className="pt-1 flex items-center justify-between gap-2 border-t border-amber-200/70 dark:border-amber-800/70">
              <span className="text-[11px] text-amber-800 dark:text-amber-300">
                Want to publish this video as a standard video instead?
              </span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={onSwitchToNormalVideo}
                className="h-7 text-xs border-amber-300 bg-white hover:bg-amber-100 text-amber-900 dark:border-amber-700 dark:bg-amber-900 dark:hover:bg-amber-800 dark:text-amber-100 shrink-0"
              >
                Switch to Normal Video
                <ArrowRight className="ml-1 h-3 w-3" />
              </Button>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
