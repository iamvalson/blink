"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CalendarClock, Copy, RefreshCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { PostDetailsPreview } from "./post-details-preview";
import { usePost } from "@/features/posts/hooks/use-posts";
import { cancelSchedule, deletePost, retryPost } from "@/features/posts/api/create-post";
import { ApiError } from "@/lib/api/client";
import { Button, buttonVariants } from "@/components/ui/button";
import { FormatBadge, StatusBadge } from "@/components/posts/status-badge";
import { PublishingProgress } from "@/components/posts/publishing-progress";
import { ReschedulePostDialog } from "@/components/posts/reschedule-post-dialog";

function dateLabel(value?: string | null) {
  return value ? new Date(value).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : "—";
}

export function PostDetails({ id }: { id: string }) {
  const postQuery = usePost(id);
  const queryClient = useQueryClient();
  const router = useRouter();
  const [rescheduling, setRescheduling] = useState(false);
  const action = useMutation({
    mutationFn: async (kind: "retry" | "cancel" | "delete") => {
      if (kind === "retry") return retryPost(id);
      if (kind === "cancel") return cancelSchedule(id);
      await deletePost(id);
    },
    onSuccess: async (updated, kind) => {
      if (kind === "delete") {
        queryClient.removeQueries({ queryKey: ["posts", id], exact: true });
        router.replace("/posts");
      } else if (updated) {
        queryClient.setQueryData(["posts", id], updated);
      }
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["posts"] }),
        queryClient.invalidateQueries({ queryKey: ["calendar"] }),
      ]);
      toast.success(kind === "retry" ? "Post requeued for publishing." : kind === "cancel" ? "Schedule cancelled." : "Post deleted.");
    },
    onError: (error) => {
      toast.error(error instanceof Error ? error.message : "Could not update the post.");
      void queryClient.invalidateQueries({ queryKey: ["posts"] });
    },
  });

  const back = <Link href="/posts" className="hidden items-center gap-2 text-sm text-muted-foreground hover:text-foreground lg:inline-flex"><ArrowLeft className="size-4" />Back to posts</Link>;
  if (postQuery.isPending) return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6">{back}<div role="status" className="rounded-xl border px-4 py-10 text-center sm:p-12 text-sm text-muted-foreground">Loading post details…</div></div>;
  if (postQuery.isError || !postQuery.data) {
    const missing = postQuery.error instanceof ApiError && postQuery.error.statusCode === 404;
    return <div className="mx-auto max-w-7xl space-y-6 px-4 py-6 sm:px-6">{back}<div role="alert" className="rounded-xl border px-4 py-10 text-center sm:p-12"><h1 className="text-xl font-semibold">{missing ? "Post not found" : "Could not load post"}</h1><p className="mb-5 mt-2 text-sm text-muted-foreground">{missing ? "This post may have been deleted or is no longer available." : "Please try loading the post again."}</p>{!missing && <Button variant="outline" onClick={() => postQuery.refetch()}>Try again</Button>}</div></div>;
  }
  const post = postQuery.data;
  const instagramFormat = post.targets?.find(target => target.platform === "instagram")?.instagram_format;
  const canReschedule = (post.status === "SCHEDULED" || post.status === "QUEUED") &&
    !post.targets?.some(target => target.send_to_tiktok_inbox);
  const canDelete = !["QUEUED", "PUBLISHING", "PROCESSING"].includes(post.status);
  const canRetry = post.status === "FAILED" || post.status === "PARTIALLY_PUBLISHED";
  const needsReconnect = post.targets?.some(target => target.error_code === "ACCOUNT_RECONNECT_REQUIRED" || /invalid_grant|refresh token is invalid or revoked/i.test(target.error_message ?? ""));

  return (
    <div className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 sm:px-6 lg:px-8">
      {back}
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0"><h1 className="break-words text-2xl font-semibold tracking-tight">Post details</h1><div className="mt-3 flex flex-wrap items-center gap-2"><StatusBadge status={post.status} /><FormatBadge format={instagramFormat ?? post.content_format} /></div></div>
        <div className="flex flex-wrap gap-2 [&_button]:min-h-11 lg:[&_button]:min-h-0">
          <Button variant="outline" size="sm" disabled={postQuery.isFetching} onClick={() => postQuery.refetch()}><RefreshCcw className="mr-2 size-4" />Refresh</Button>
          {canReschedule && <Button variant="outline" size="sm" disabled={action.isPending} onClick={() => setRescheduling(true)}><CalendarClock className="mr-2 size-4" />Reschedule</Button>}
          {post.status === "SCHEDULED" && <Button variant="outline" size="sm" disabled={action.isPending} onClick={() => { if (window.confirm("Cancel this post's schedule? It will not be published.")) action.mutate("cancel"); }}>Cancel schedule</Button>}
          {canDelete && <Button variant="outline" size="sm" disabled={action.isPending} onClick={() => { if (window.confirm("Delete this post? This action cannot be undone.")) action.mutate("delete"); }}><Trash2 className="mr-2 size-4" />Delete</Button>}
        </div>
      </div>
      {rescheduling && canReschedule && <ReschedulePostDialog post={post} onClose={() => setRescheduling(false)} />}
      <div className="grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
        <PostDetailsPreview key={post.id} post={post} />
        <div className="min-w-0 space-y-6">
          <section aria-label="Publishing status">
            <PublishingProgress showDetailsLink={false} post={post} onRetry={canRetry ? () => action.mutate("retry") : undefined} isRetrying={action.isPending} />
            {needsReconnect && <div className="mt-3 rounded-lg border border-amber-300 bg-amber-50 p-4 text-sm text-amber-900"><p className="mb-3">An account needs to be reconnected before publishing can succeed.</p><Link href="/accounts" className={buttonVariants({ size: "sm", variant: "outline" })}>Reconnect account</Link></div>}
            {!post.targets?.length && <p className="mt-3 text-sm text-muted-foreground">No publishing destinations are available for this post.</p>}
          </section>
          <section className="rounded-xl border bg-card p-5">
            <h2 className="mb-4 font-semibold">Post information</h2>
            <dl className="space-y-4 text-sm">
              {[["Created", post.created_at], ["Last updated", post.updated_at], ["Scheduled for", post.scheduled_at], ["Published", post.published_at]].map(([label, value]) => <div key={label}><dt className="text-xs text-muted-foreground">{label}</dt><dd className="mt-1">{dateLabel(value)}</dd></div>)}
              <div><dt className="text-xs text-muted-foreground">Post ID</dt><dd className="mt-1 flex items-center gap-2"><span className="break-all font-mono text-xs">{post.id}</span><button type="button" aria-label="Copy post ID" className="flex size-11 shrink-0 items-center justify-center rounded hover:bg-muted lg:size-8" onClick={async () => { try { await navigator.clipboard.writeText(post.id); toast.success("Post ID copied."); } catch { toast.error("Could not copy the post ID."); } }}><Copy className="size-3.5" /></button></dd></div>
            </dl>
          </section>
        </div>
      </div>
    </div>
  );
}
