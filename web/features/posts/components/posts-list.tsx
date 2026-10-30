"use client";

import { deletePost, retryPost } from "@/features/posts/api/create-post";
import { usePosts, postsQueryKey } from "@/features/posts/hooks/use-posts";
import { usePostStatusStream } from "@/features/posts/hooks/use-post-status-stream";
import type { ListPostsFilter, PostResponse } from "@/types/post";
import {
  AlertCircle,
  ExternalLink,
  FileText,
  Loader2,
  MoreHorizontal,
  Plus,
  RefreshCcw,
  Trash2,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { siTiktok, siX, siYoutube } from "simple-icons";
import { formatDistanceToNow } from "date-fns";

import { Button, buttonVariants } from "@/components/ui/button";
import { FormatBadge, StatusBadge } from "@/components/posts/status-badge";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

// ─── Helpers ─────────────────────────────────────────────────────────────────

const TERMINAL_STATUSES = new Set([
  "PUBLISHED",
  "PARTIALLY_PUBLISHED",
  "FAILED",
  "CANCELLED",
]);

// ─── Platform icon ────────────────────────────────────────────────────────────

function PlatformIcon({
  platform,
  className = "h-3.5 w-3.5",
}: {
  platform: string;
  className?: string;
}) {
  const icon =
    platform === "youtube"
      ? siYoutube
      : platform === "twitter"
        ? siX
        : platform === "tiktok"
          ? siTiktok
          : null;
  if (!icon) return null;
  return (
    <svg
      role="img"
      viewBox="0 0 24 24"
      className={`fill-current ${className}`}
      aria-hidden
    >
      <path d={icon.path} />
    </svg>
  );
}

// ─── Status filter tabs ───────────────────────────────────────────────────────

const STATUS_FILTERS = [
  { value: "", label: "All" },
  { value: "QUEUED", label: "Queued" },
  { value: "PUBLISHING", label: "Publishing" },
  { value: "PUBLISHED", label: "Published" },
  { value: "FAILED", label: "Failed" },
];

const PLATFORM_FILTERS = [
  { value: "", label: "All platforms" },
  { value: "youtube", label: "YouTube" },
  { value: "twitter", label: "X" },
  { value: "tiktok", label: "TikTok" },
];

// ─── Post card ────────────────────────────────────────────────────────────────

function PostCard({
  post,
  onDeleted,
  onRetried,
}: {
  post: PostResponse;
  onDeleted: () => void;
  onRetried: () => void;
}) {
  const router = useRouter();
  const [isDeleting, setIsDeleting] = useState(false);
  const [isRetrying, setIsRetrying] = useState(false);

  // Open an SSE stream for every non-terminal post.
  // usePostStatusStream invalidates postsQueryKey() on every status event,
  // which re-fetches the list query — giving us live status changes.
  const isTerminal = TERMINAL_STATUSES.has(post.status);
  usePostStatusStream(post.id, !isTerminal);

  const caption = post.caption ?? post.title ?? "(no caption)";
  const platform = post.targets?.[0]?.platform ?? "unknown";
  const platformUrl = post.targets?.find((t) => t.platform_url)?.platform_url;
  const failedTarget = post.targets?.find((t) => t.status === "FAILED");
  const isReconnectRequired =
    failedTarget?.error_code === "ACCOUNT_RECONNECT_REQUIRED" ||
    failedTarget?.error_message?.toLowerCase().includes("refresh token is invalid or revoked") ||
    failedTarget?.error_message?.toLowerCase().includes("invalid_grant");

  const reconnectPlatform = failedTarget?.platform || platform;

  const handleReconnect = () => {
    const apiUrl = process.env.NEXT_PUBLIC_API_URL || "";
    toast.info(`Redirecting to reconnect ${reconnectPlatform === "twitter" ? "X" : reconnectPlatform}...`);
    window.location.href = `${apiUrl}/auth/${reconnectPlatform}`;
  };

  const canDelete =
    post.status !== "QUEUED" && post.status !== "PUBLISHING" && post.status !== "PROCESSING";
  const canRetry = post.status === "FAILED";

  const handleDelete = async () => {
    if (!confirm("Delete this post? This action cannot be undone.")) return;
    try {
      setIsDeleting(true);
      await deletePost(post.id);
      toast.success("Post deleted.");
      onDeleted();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Could not delete the post.";
      toast.error(msg);
    } finally {
      setIsDeleting(false);
    }
  };

  const handleRetry = async () => {
    try {
      setIsRetrying(true);
      await retryPost(post.id);
      toast.success("Post requeued for publishing.");
      onRetried();
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Could not retry the post.";
      toast.error(msg);
    } finally {
      setIsRetrying(false);
    }
  };

  return (
    <div className="flex items-start gap-4 rounded-lg border bg-card p-4 hover:bg-muted/30 transition-colors">
      {/* Platform icon */}
      <div className="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-background">
        <PlatformIcon platform={platform} />
      </div>

      {/* Main content */}
      <div className="min-w-0 flex-1 space-y-1.5">
        <div className="flex flex-wrap items-start gap-2">
          <p className="text-sm font-medium leading-snug line-clamp-2 flex-1">
            {caption}
          </p>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <StatusBadge status={post.status} />
          {post.content_format && post.content_format !== "VIDEO" && (
            <FormatBadge format={post.content_format} />
          )}
          {post.targets && post.targets.length > 1 && (
            <span className="text-xs text-muted-foreground">
              {post.targets.length} targets
            </span>
          )}
        </div>

        {/* Per-target status for multi-platform posts */}
        {post.targets && post.targets.length > 1 && (
          <div className="flex flex-wrap gap-2 mt-1">
            {post.targets.map((target) => (
              <div
                key={target.id}
                className="flex items-center gap-1 text-xs text-muted-foreground"
              >
                <PlatformIcon
                  platform={target.platform}
                  className="h-3 w-3 fill-current"
                />
                <span className="capitalize">
                  {target.platform === "twitter"
                    ? "X"
                    : target.platform === "tiktok"
                      ? "TikTok"
                      : target.platform}
                </span>
                <StatusBadge status={target.status} />
              </div>
            ))}
          </div>
        )}

        {/* Error / reconnect */}
        {isReconnectRequired ? (
          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-2 rounded-md bg-amber-500/10 border border-amber-500/20 p-2.5 text-xs text-amber-800 dark:text-amber-300">
            <div className="flex items-center gap-1.5 font-medium">
              <AlertCircle className="h-4 w-4 shrink-0 text-amber-600 dark:text-amber-400" />
              <span>
                {reconnectPlatform === "twitter" ? "X" : reconnectPlatform.toUpperCase()} authorization expired. Reconnect Blink to publish.
              </span>
            </div>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="h-7 text-xs border-amber-500/30 hover:bg-amber-500/10 text-amber-900 dark:text-amber-200"
              onClick={handleReconnect}
            >
              Reconnect {reconnectPlatform === "twitter" ? "X" : reconnectPlatform}
            </Button>
          </div>
        ) : failedTarget?.error_message ? (
          <p className="text-xs text-red-600 line-clamp-2">
            {failedTarget.error_message}
          </p>
        ) : null}

        <p className="text-xs text-muted-foreground">
          {formatDistanceToNow(new Date(post.created_at), { addSuffix: true })}
        </p>
      </div>

      {/* Actions */}
      <div className="flex shrink-0 items-center gap-1">
        {platformUrl && (
          <a
            href={platformUrl}
            target="_blank"
            rel="noopener noreferrer"
            className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            aria-label="View on platform"
          >
            <ExternalLink className="h-4 w-4" />
          </a>
        )}

        <DropdownMenu>
          <DropdownMenuTrigger
            className="flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground transition-colors"
            aria-label="Post actions"
          >
            <MoreHorizontal className="h-4 w-4" />
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => router.push(`/posts/${post.id}`)}>
              View details
            </DropdownMenuItem>

            {isReconnectRequired && (
              <DropdownMenuItem
                onClick={handleReconnect}
                className="gap-2 text-amber-600 dark:text-amber-400 focus:text-amber-700"
              >
                <RefreshCcw className="h-3.5 w-3.5" />
                Reconnect {reconnectPlatform === "twitter" ? "X" : reconnectPlatform}
              </DropdownMenuItem>
            )}

            {canRetry && (
              <DropdownMenuItem
                onClick={handleRetry}
                disabled={isRetrying}
                className="gap-2"
              >
                {isRetrying ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RefreshCcw className="h-3.5 w-3.5" />
                )}
                Retry publish
              </DropdownMenuItem>
            )}

            {platformUrl && (
              <DropdownMenuItem
                onClick={() => window.open(platformUrl, "_blank", "noopener,noreferrer")}
                className="gap-2"
              >
                <ExternalLink className="h-3.5 w-3.5" />
                View on platform
              </DropdownMenuItem>
            )}

            {canDelete && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  onClick={handleDelete}
                  disabled={isDeleting}
                  className="gap-2 text-destructive focus:text-destructive"
                >
                  {isDeleting ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="h-3.5 w-3.5" />
                  )}
                  Delete
                </DropdownMenuItem>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

// ─── Skeleton loader ──────────────────────────────────────────────────────────

function PostCardSkeleton() {
  return (
    <div className="flex items-start gap-4 rounded-lg border bg-card p-4">
      <div className="mt-0.5 h-9 w-9 shrink-0 rounded-md bg-muted animate-pulse" />
      <div className="min-w-0 flex-1 space-y-2.5">
        <div className="h-4 w-3/4 rounded bg-muted animate-pulse" />
        <div className="h-3 w-1/2 rounded bg-muted animate-pulse" />
        <div className="h-3 w-1/4 rounded bg-muted animate-pulse" />
      </div>
    </div>
  );
}

// ─── Main posts list ──────────────────────────────────────────────────────────

export function PostsList() {
  const [filter, setFilter] = useState<ListPostsFilter>({ limit: 50 });
  const queryClient = useQueryClient();

  // Primary fetch — also used as the SSE invalidation target.
  const { data, isLoading, isError, refetch } = usePosts(filter);

  const posts = data?.posts ?? [];
  const total = data?.total ?? 0;

  // Poll every 5 s as long as any visible post is still in-flight.
  // Each PostCard also opens its own SSE stream for immediate updates;
  // polling is a safety net for proxies that buffer SSE or dropped connections.
  const hasInFlight = posts.some((p) => !TERMINAL_STATUSES.has(p.status));
  usePosts(filter, hasInFlight ? 5000 : false);

  const invalidate = () => {
    queryClient.invalidateQueries({ queryKey: postsQueryKey(filter) });
  };

  return (
    <div className="space-y-6">
      {/* Filters */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        {/* Status tabs */}
        <div className="flex flex-wrap gap-1">
          {STATUS_FILTERS.map((f) => (
            <button
              key={f.value}
              onClick={() =>
                setFilter((prev) => ({
                  ...prev,
                  status: f.value || undefined,
                  offset: 0,
                }))
              }
              className={[
                "rounded-md px-3 py-1.5 text-xs font-medium transition-colors",
                (filter.status ?? "") === f.value
                  ? "bg-foreground text-background"
                  : "text-muted-foreground hover:bg-muted hover:text-foreground",
              ].join(" ")}
            >
              {f.label}
            </button>
          ))}
        </div>

        {/* Platform filter */}
        <div className="flex items-center gap-2">
          <select
            value={filter.platform ?? ""}
            onChange={(e) =>
              setFilter((prev) => ({
                ...prev,
                platform: e.target.value || undefined,
                offset: 0,
              }))
            }
            className="rounded-md border bg-background px-2.5 py-1.5 text-xs text-foreground focus:outline-none focus:ring-2 focus:ring-ring"
            aria-label="Filter by platform"
          >
            {PLATFORM_FILTERS.map((f) => (
              <option key={f.value} value={f.value}>
                {f.label}
              </option>
            ))}
          </select>

          {/* Search */}
          <input
            type="search"
            placeholder="Search posts…"
            value={filter.search ?? ""}
            onChange={(e) =>
              setFilter((prev) => ({
                ...prev,
                search: e.target.value || undefined,
                offset: 0,
              }))
            }
            className="w-40 rounded-md border bg-background px-2.5 py-1.5 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring sm:w-48"
            aria-label="Search posts"
          />
        </div>
      </div>

      {/* Results summary */}
      {!isLoading && !isError && (
        <p className="text-xs text-muted-foreground">
          {total === 0 ? "No posts" : `${total} post${total !== 1 ? "s" : ""}`}
        </p>
      )}

      {/* Loading skeleton */}
      {isLoading && (
        <div className="space-y-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <PostCardSkeleton key={i} />
          ))}
        </div>
      )}

      {/* Error state */}
      {isError && (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed p-12 text-center">
          <AlertCircle className="h-8 w-8 text-muted-foreground" />
          <div>
            <p className="font-medium">Could not load posts</p>
            <p className="mt-1 text-sm text-muted-foreground">
              There was a problem fetching your posts.
            </p>
          </div>
          <Button size="sm" variant="outline" onClick={() => refetch()}>
            <RefreshCcw className="mr-2 h-3.5 w-3.5" />
            Try again
          </Button>
        </div>
      )}

      {/* Empty state */}
      {!isLoading && !isError && posts.length === 0 && (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed p-12 text-center">
          <FileText className="h-8 w-8 text-muted-foreground" />
          <div>
            {filter.status || filter.platform || filter.search ? (
              <>
                <p className="font-medium">No matching posts</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Try adjusting your filters.
                </p>
              </>
            ) : (
              <>
                <p className="font-medium">No posts yet</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Create your first post to get started.
                </p>
              </>
            )}
          </div>
          <Link href="/posts/new" className={buttonVariants({ size: "sm" })}>
            <Plus className="mr-2 h-3.5 w-3.5" />
            Create post
          </Link>
        </div>
      )}

      {/* Post list */}
      {!isLoading && !isError && posts.length > 0 && (
        <div className="space-y-3">
          {posts.map((post) => (
            <PostCard
              key={post.id}
              post={post}
              onDeleted={invalidate}
              onRetried={invalidate}
            />
          ))}
        </div>
      )}
    </div>
  );
}
