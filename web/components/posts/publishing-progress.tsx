"use client";

import { cn } from "@/lib/utils";
import type { PostResponse } from "@/types/post";
import {
  AlertCircle,
  CheckCircle2,
  Clock,
  ExternalLink,
  Loader2,
  RefreshCcw,
} from "lucide-react";
import Link from "next/link";
import { siYoutube, siX, siTiktok } from "simple-icons";
import { Button, buttonVariants } from "@/components/ui/button";

interface PublishingProgressProps {
  post: PostResponse;
  onRetry?: () => void;
  isRetrying?: boolean;
  showDetailsLink?: boolean;
}

function PlatformIcon({ platform }: { platform: string }) {
  if (platform === "youtube") {
    return (
      <svg
        role="img"
        viewBox="0 0 24 24"
        className="h-3.5 w-3.5 fill-current"
        aria-hidden
      >
        <path d={siYoutube.path} />
      </svg>
    );
  }
  if (platform === "twitter") {
    return (
      <svg
        role="img"
        viewBox="0 0 24 24"
        className="h-3.5 w-3.5 fill-current"
        aria-hidden
      >
        <path d={siX.path} />
      </svg>
    );
  }
  if (platform === "tiktok") {
    return (
      <svg
        role="img"
        viewBox="0 0 24 24"
        className="h-3.5 w-3.5 fill-current"
        aria-hidden
      >
        <path d={siTiktok.path} />
      </svg>
    );
  }
  return null;
}

function platformDisplayName(platform: string): string {
  if (platform === "twitter") return "X";
  if (platform === "tiktok") return "TikTok";
  return platform.charAt(0).toUpperCase() + platform.slice(1);
}

function StatusBadge({ status }: { status: string }) {
  const config: Record<
    string,
    { label: string; className: string; icon: React.ElementType | null }
  > = {
    QUEUED: {
      label: "Queued",
      className: "bg-blue-50 text-blue-700 border-blue-200",
      icon: Clock,
    },
    PENDING: {
      label: "Pending",
      className: "bg-blue-50 text-blue-700 border-blue-200",
      icon: Clock,
    },
    PROCESSING: {
      label: "Processing",
      className: "bg-amber-50 text-amber-700 border-amber-200",
      icon: Loader2,
    },
    PUBLISHING: {
      label: "Publishing",
      className: "bg-amber-50 text-amber-700 border-amber-200",
      icon: Loader2,
    },
    INBOX_DELIVERED: {
      label: "Sent to TikTok Inbox",
      className: "bg-emerald-50 text-emerald-700 border-emerald-200",
      icon: CheckCircle2,
    },
    PUBLISHED: {
      label: "Published",
      className: "bg-emerald-50 text-emerald-700 border-emerald-200",
      icon: CheckCircle2,
    },
    FAILED: {
      label: "Failed",
      className: "bg-red-50 text-red-700 border-red-200",
      icon: AlertCircle,
    },
    UNKNOWN: {
      label: "Needs review",
      className: "bg-neutral-50 text-neutral-600 border-neutral-200",
      icon: RefreshCcw,
    },
    RETRYING: {
      label: "Retrying",
      className: "bg-amber-50 text-amber-700 border-amber-200",
      icon: RefreshCcw,
    },
    CANCELLED: {
      label: "Cancelled",
      className: "bg-neutral-50 text-neutral-600 border-neutral-200",
      icon: null,
    },
  };

  const cfg = config[status] ?? {
    label: status,
    className: "bg-neutral-50 text-neutral-600 border-neutral-200",
    icon: null,
  };
  const Icon = cfg.icon;
  const isSpinning = status === "PROCESSING" || status === "PUBLISHING";

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium",
        cfg.className,
      )}
    >
      {Icon && (
        <Icon
          className={cn("h-3 w-3", isSpinning && "animate-spin")}
          aria-hidden
        />
      )}
      {cfg.label}
    </span>
  );
}

export function PublishingProgress({
  post,
  onRetry,
  isRetrying,
  showDetailsLink = true,
}: PublishingProgressProps) {
  const isActive =
    post.status === "QUEUED" ||
    post.status === "PUBLISHING" ||
    post.status === "PROCESSING";
  const isFailed = post.status === "FAILED" || post.status === "PARTIALLY_PUBLISHED";
  const isPublished = post.status === "PUBLISHED";
  const isInboxDelivered = post.status === "INBOX_DELIVERED";
  const isInboxUpload = post.targets?.some(target => target.send_to_tiktok_inbox);

  return (
    <div
      className={cn(
        "rounded-lg border p-4 space-y-4 text-sm",
        isActive && "border-amber-200 bg-amber-50/50",
        isFailed && "border-red-200 bg-red-50/50",
        (isPublished || isInboxDelivered) && "border-emerald-200 bg-emerald-50/50",
        !isActive && !isFailed && !isPublished && !isInboxDelivered && "border-border bg-muted/30",
      )}
    >
      <div className="flex items-center justify-between gap-3">
        <div>
          <p className="font-medium text-foreground">Publishing status</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            Post ID: {post.id.slice(0, 8)}…
          </p>
        </div>
        <StatusBadge status={post.status} />
      </div>

      {isInboxDelivered && (
        <p>Open the notification in your TikTok Inbox to edit and post the video.</p>
      )}

      {/* Per-target status */}
      {post.targets && post.targets.length > 0 && (
        <div className="space-y-2">
          {post.targets.map((target) => (
            <div
              key={target.id}
              className="flex items-center justify-between gap-2 rounded-md border bg-background px-3 py-2"
            >
              <div className="flex items-center gap-2 min-w-0">
                <PlatformIcon platform={target.platform} />
                <span className="text-xs font-medium truncate">
                  {platformDisplayName(target.platform)}
                </span>
                {target.platform_user_id && (
                  <span className="text-xs text-muted-foreground truncate">
                    @{target.platform_user_id}
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2 shrink-0">
                <StatusBadge status={target.status} />
                {target.platform_url && (
                  <a
                    href={target.platform_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-primary hover:text-primary/80 transition-colors"
                    aria-label="View on platform"
                  >
                    <ExternalLink className="h-3.5 w-3.5" />
                  </a>
                )}
              </div>
            </div>
          ))}
        </div>
      )}

      {post.targets?.filter(t => t.platform === "tiktok" && t.status === "UNKNOWN").map(t => <p key={t.id} role="alert" className="text-amber-800">{t.error_message || "TikTok publication needs manual review. Check TikTok before attempting another post."}</p>)}
      {post.targets?.some(t => t.platform === "tiktok" && t.status === "PROCESSING") && <p role="status">{isInboxUpload ? "TikTok is processing your video for Inbox delivery." : "TikTok is processing your video. Publication is awaiting confirmation."}</p>}
      {post.targets?.filter(t => t.status === "RETRYING" && t.error_message).map(t => (
        <p key={t.id} role="status" className="text-amber-800">
          {platformDisplayName(t.platform)} retry scheduled: {t.error_message}
        </p>
      ))}
      {/* Error details for failed targets */}
      {post.targets?.some((t) => t.status === "FAILED") && (
        <div className="space-y-1.5">
          {post.targets
            .filter((t) => t.status === "FAILED" && t.error_message)
            .map((t) => (
              <div
                key={t.id}
                className="rounded-md border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700"
              >
                <span className="font-medium">
                  {platformDisplayName(t.platform)}:
                </span>{" "}
                {t.error_message}
              </div>
            ))}
        </div>
      )}

      {/* Indeterminate progress indicator */}
      {isActive && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3 w-3 animate-spin shrink-0" />
          <span>
            {isInboxUpload ? "Sending to TikTok Inbox…" : post.status === "QUEUED"
              ? "Waiting to publish…"
              : "Uploading and publishing…"}
          </span>
        </div>
      )}

      {/* Success message with link */}
      {isPublished &&
        post.targets?.some((t) => t.platform_url) && (
          <div className="flex flex-wrap gap-2">
            {post.targets
              .filter((t) => t.platform_url)
              .map((t) => (
                <a
                  key={t.id}
                  href={t.platform_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                >
                  View on {platformDisplayName(t.platform)}
                  <ExternalLink className="h-3 w-3" />
                </a>
              ))}
          </div>
        )}

      {/* Actions */}
      <div className="flex flex-wrap gap-2">
        {showDetailsLink && <Link href={`/posts/${post.id}`} className={buttonVariants({ size: "sm", variant: "outline", className: "text-xs h-7" })}>View details</Link>}
        {isFailed && onRetry && (
          <Button
            size="sm"
            variant="outline"
            onClick={onRetry}
            disabled={isRetrying}
            className="text-xs h-7"
          >
            {isRetrying ? (
              <Loader2 className="mr-1.5 h-3 w-3 animate-spin" />
            ) : (
              <RefreshCcw className="mr-1.5 h-3 w-3" />
            )}
            {isRetrying ? "Retrying…" : "Retry publish"}
          </Button>
        )}
        <Link
          href="/posts"
          className={buttonVariants({
            size: "sm",
            variant: "ghost",
            className: "text-xs h-7",
          })}
        >
          View all posts
        </Link>
      </div>
    </div>
  );
}
