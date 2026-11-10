"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/api/client";
import { PostPreview, type PreviewMedia } from "@/components/posts/post-preview";
import { PLATFORMS, type Platform } from "@/types/platform";
import type { PostResponse } from "@/types/post";

export function PostDetailsPreview({ post }: { post: PostResponse }) {
  const platforms = Array.from(new Set((post.targets ?? []).map(target => target.platform)))
    .filter((platform): platform is Platform => PLATFORMS.some(entry => entry.id === platform));
  const [selectedPlatform, setSelectedPlatform] = useState<Platform | undefined>(platforms[0]);
  const platform = selectedPlatform && platforms.includes(selectedPlatform) ? selectedPlatform : platforms[0];
  const account = post.targets?.find(target => target.platform === platform);
  const instagramMedia = platform === "instagram" ? post.instagram_media : undefined;
  const loadOriginal = Boolean(post.media_id && !instagramMedia);
  const mediaBase = process.env.NEXT_PUBLIC_API_URL ?? "";
  const media = useQuery({
    queryKey: ["media", post.media_id],
    queryFn: () => apiClient<{ content_type: string; upload_status: string }>(`/api/v1/media/${post.media_id}`),
    enabled: loadOriginal,
  });
  const attachments: PreviewMedia[] = instagramMedia?.map(item => ({
    id: `instagram-${item.position}`,
    previewUrl: item.media_id ? `${mediaBase}/api/v1/media/${item.media_id}/content` : "",
    contentType: item.content_type,
    label: `Attachment ${item.position + 1}`,
  })) ?? [];
  // Prefer durable uploads over any legacy URL, which may have expired.
  if (post.media_id && loadOriginal && media.data?.upload_status === "complete") {
    attachments.push({
      id: post.media_id,
      previewUrl: `${mediaBase}/api/v1/media/${post.media_id}/content`,
      contentType: media.data.content_type,
    });
  } else if (!instagramMedia && !post.media_id && post.media_url) {
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
              {PLATFORMS.find(entry => entry.id === value)?.name}
            </button>
          ))}
        </div>
      )}
      <PostPreview
        saved
        title={account?.instagram_format !== "STORIES" && account?.caption_override === undefined ? post.title : undefined}
        caption={account?.instagram_format === "STORIES" ? "" : account?.caption_override ?? post.caption ?? ""}
        media={attachments}
        platforms={platform ? [platform] : []}
        accountName={account?.platform_username}
        accountAvatarUrl={account?.platform_avatar_url}
        date={post.published_at ?? post.scheduled_at ?? post.created_at}
      />
      {loadOriginal && media.isPending && <p role="status" className="text-sm text-muted-foreground">Loading attached media…</p>}
      {loadOriginal && media.isError && (
        <div role="alert" className="rounded-lg border p-4 text-sm">
          <p>The attached media could not be loaded.</p>
          <button type="button" className="mt-2 font-medium underline" onClick={() => media.refetch()}>Retry media</button>
        </div>
      )}
      {loadOriginal && media.data && media.data.upload_status !== "complete" && <p role="status" className="text-sm text-muted-foreground">This media upload is not available for preview.</p>}
    </section>
  );
}
