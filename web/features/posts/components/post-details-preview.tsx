"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api/client";
import { PostPreview, type PreviewMedia } from "@/components/posts/post-preview";
import type { Platform } from "@/components/posts/platform-selector";
import type { PostResponse } from "@/types/post";

export function PostDetailsPreview({ post }: { post: PostResponse }) {
  const platforms = Array.from(new Set((post.targets ?? []).map(target => target.platform)))
    .filter((platform): platform is Platform => ["twitter", "youtube", "tiktok"].includes(platform));
  const [selectedPlatform, setSelectedPlatform] = useState<Platform | undefined>(platforms[0]);
  const platform = selectedPlatform && platforms.includes(selectedPlatform) ? selectedPlatform : platforms[0];
  const account = post.targets?.find(target => target.platform === platform);
  const media = useQuery({
    queryKey: ["media", post.media_id],
    queryFn: () => apiClient<{ content_type: string; upload_status: string }>(`/api/v1/media/${post.media_id}`),
    enabled: Boolean(post.media_id),
  });
  const attachments: PreviewMedia[] = [];
  // Prefer durable uploads over any legacy URL, which may have expired.
  if (post.media_id && media.data?.upload_status === "complete") {
    attachments.push({
      id: post.media_id,
      previewUrl: `${process.env.NEXT_PUBLIC_API_URL}/api/v1/media/${post.media_id}/content`,
      contentType: media.data.content_type,
    });
  } else if (!post.media_id && post.media_url) {
    attachments.push({ id: post.id, previewUrl: post.media_url, contentType: post.media_type ?? undefined });
  }

  return (
    <section className="min-w-0 space-y-3" aria-label="Post preview">
      {platforms.length > 1 && (
        <div role="group" aria-label="Preview platform" className="flex flex-wrap gap-2">
          {platforms.map(value => (
            <button
              key={value}
              type="button"
              aria-pressed={platform === value}
              onClick={() => setSelectedPlatform(value)}
              className={`rounded-md border px-3 py-1.5 text-xs font-medium ${platform === value ? "bg-foreground text-background" : "hover:bg-muted"}`}
            >
              {value === "twitter" ? "X" : value === "tiktok" ? "TikTok" : "YouTube"}
            </button>
          ))}
        </div>
      )}
      <PostPreview
        saved
        title={account?.caption_override === undefined ? post.title : undefined}
        caption={account?.caption_override ?? post.caption ?? ""}
        media={attachments}
        platforms={platform ? [platform] : []}
        accountName={account?.platform_user_id}
        date={post.published_at ?? post.scheduled_at ?? post.created_at}
      />
      {post.media_id && media.isPending && <p role="status" className="text-sm text-muted-foreground">Loading attached media…</p>}
      {post.media_id && media.isError && (
        <div role="alert" className="rounded-lg border p-4 text-sm">
          <p>The attached media could not be loaded.</p>
          <button type="button" className="mt-2 font-medium underline" onClick={() => media.refetch()}>Retry media</button>
        </div>
      )}
      {post.media_id && media.data && media.data.upload_status !== "complete" && <p role="status" className="text-sm text-muted-foreground">This media upload is not available for preview.</p>}
    </section>
  );
}
