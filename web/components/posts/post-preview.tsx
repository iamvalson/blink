"use client";

import { ImageIcon, MoreHorizontal } from "lucide-react";
import { useState } from "react";
import { PlatformIcon } from "@/components/accounts/platform-icon";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { PLATFORMS, type Platform } from "@/types/platform";

export type PreviewMedia = {
  id: string;
  previewUrl: string;
  file?: { type: string };
  contentType?: string;
  label?: string;
};

type PostPreviewProps = {
  caption: string;
  media: PreviewMedia[];
  platforms: Platform[];
  title?: string | null;
  accountName?: string;
  accountAvatarUrl?: string;
  saved?: boolean;
  date?: string;
};

function MediaPreview({ item }: { item: PreviewMedia }) {
  const [failed, setFailed] = useState(false);
  const contentType = item.contentType ?? item.file?.type ?? "";
  if (!item.previewUrl) {
    return <p className="rounded-lg bg-muted p-4 text-sm text-muted-foreground">{item.label ?? "Attachment"} is unavailable.</p>;
  }
  if (failed) {
    return (
      <div role="status" className="rounded-lg bg-muted p-6 text-center text-sm text-muted-foreground">
        <p>This media could not be loaded.</p>
        <button type="button" className="mt-2 font-medium underline" onClick={() => setFailed(false)}>Retry media</button>
      </div>
    );
  }
  if (contentType.startsWith("video")) {
    return (
      <video
        src={item.previewUrl}
        aria-label={item.label ?? "Post attachment"}
        controls
        playsInline
        preload="metadata"
        onError={() => setFailed(true)}
        className="max-h-[520px] w-full bg-black object-contain"
      >
        Your browser does not support video playback.
      </video>
    );
  }
  if (contentType.startsWith("image")) {
    return (
      // Uploaded previews can be authenticated API URLs or local browser blobs.
      // eslint-disable-next-line @next/next/no-img-element
      <img
        src={item.previewUrl}
        alt={item.label ?? "Post attachment"}
        onError={() => setFailed(true)}
        className="max-h-[520px] w-full object-contain"
      />
    );
  }
  return (
    <a href={item.previewUrl} target="_blank" rel="noopener noreferrer" className="block p-4 text-sm hover:underline">
      Open attached media →
    </a>
  );
}

function ConnectedAccountAvatar({ name, url }: { name?: string; url?: string }) {
  const [failed, setFailed] = useState(false);
  return (
    <div className="flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-full bg-muted text-sm font-semibold" aria-hidden>
      {url?.startsWith("https://") && !failed ? (
        // The provider hosts this public avatar. Avoid sending the Blink page URL as the referrer.
        // eslint-disable-next-line @next/next/no-img-element
        <img src={url} alt="" referrerPolicy="no-referrer" onError={() => setFailed(true)} className="size-full object-cover" />
      ) : (name?.trim().slice(0, 1).toUpperCase() || "B")}
    </div>
  );
}

export function PostPreview({ caption, media, platforms, title, accountName, accountAvatarUrl, saved = false, date }: PostPreviewProps) {
  const previewPlatform = platforms[0];
  const platformName = PLATFORMS.find(platform => platform.id === previewPlatform)?.name;
  const displayName = accountName || "Your account";
  const [firstLine, ...remainingLines] = caption.split("\n");
  const usesCaptionTitle = previewPlatform === "youtube" && (!title || title === firstLine.trim());
  const displayTitle = usesCaptionTitle ? firstLine.trim() : title;
  const displayCaption = usesCaptionTitle ? remainingLines.join("\n").trim() : caption;

  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b px-5 py-4">
        <div>
          <h2 className="text-sm font-medium">{saved ? "Post preview" : "Preview"}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {saved ? (platformName ? `Content preview for ${platformName}. Platform layouts may vary.` : "Preview of saved content.") : "See how your post will look."}
          </p>
        </div>
      </CardHeader>
      <CardContent className="p-3 sm:p-5">
        <div className="overflow-hidden rounded-xl border bg-background shadow-sm">
          <div className="flex items-center justify-between gap-3 border-b px-4 py-3">
            <div className="flex min-w-0 items-center gap-3">
              <ConnectedAccountAvatar key={accountAvatarUrl} name={accountName} url={accountAvatarUrl} />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{displayName}</p>
                <p className="flex items-center gap-1 text-xs text-muted-foreground">
                  {saved && previewPlatform && <PlatformIcon platform={previewPlatform} size={12} decorative />}
                  {saved ? (platformName ?? "Saved post") : previewPlatform === "youtube" ? "Your channel" : "@youraccount"}
                </p>
              </div>
            </div>
            <MoreHorizontal className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          </div>
          <div className="space-y-4 p-4">
            {displayTitle && (usesCaptionTitle || displayTitle !== firstLine.trim()) && <h3 className="wrap-anywhere font-semibold">{displayTitle}</h3>}
            {(displayCaption || (!saved && !displayTitle)) && (
              <p className="whitespace-pre-wrap wrap-anywhere text-sm leading-6">
                {displayCaption || <span className="text-muted-foreground">Your post caption will appear here...</span>}
              </p>
            )}
            {media.length > 0 ? (
              <div className="grid grid-cols-1 gap-2 overflow-hidden rounded-lg">
                {media.map(item => <MediaPreview key={`${item.id}:${item.previewUrl}`} item={item} />)}
              </div>
            ) : !saved ? (
              <div className="flex h-32 items-center justify-center rounded-lg bg-muted/50">
                {previewPlatform === "youtube" || previewPlatform === "tiktok" ? (
                  <PlatformIcon platform={previewPlatform} size={24} className="text-muted-foreground" />
                ) : <ImageIcon className="size-6 text-muted-foreground" />}
              </div>
            ) : !caption && !title ? <p className="text-sm text-muted-foreground">No content is available for this post.</p> : null}
            <div className="flex flex-wrap items-center justify-between gap-2 border-t pt-3 text-xs text-muted-foreground">
              <span>{date ? new Date(date).toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" }) : saved ? "Saved post" : "Now"}</span>
              {platformName && <span>{saved ? platformName : `Post on ${platformName}`}</span>}
            </div>
          </div>
        </div>
        {platforms.length > 1 && <p className="mt-4 text-center text-xs text-muted-foreground">Previewing {platformName} · {platforms.length} platforms selected</p>}
      </CardContent>
    </Card>
  );
}
