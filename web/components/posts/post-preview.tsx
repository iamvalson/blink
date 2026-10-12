"use client";

import { ImageIcon, MoreHorizontal } from "lucide-react";
import { siYoutube } from "simple-icons";

import { Card, CardContent, CardHeader } from "@/components/ui/card";

import type { Platform } from "./platform-selector";
import type { MediaFile } from "./media-uploader";

type PostPreviewProps = {
  caption: string;
  media: MediaFile[];
  platforms: Platform[];
};

export function PostPreview({ caption, media, platforms }: PostPreviewProps) {
  const previewPlatform = platforms[0] ?? "twitter";

  return (
    <Card className="overflow-hidden">
      <CardHeader className="border-b px-5 py-4">
        <div>
          <p className="text-sm font-medium">Preview</p>
          <p className="mt-1 text-xs text-muted-foreground">
            See how your post will look.
          </p>
        </div>
      </CardHeader>

      <CardContent className="p-5">
        <div className="overflow-hidden rounded-xl border bg-background shadow-sm">
          <div className="flex items-center justify-between border-b px-4 py-3">
            <div className="flex items-center gap-3">
              <div className="flex h-9 w-9 items-center justify-center rounded-full bg-muted text-sm font-semibold">
                B
              </div>

              <div>
                <p className="text-sm font-medium">Your account</p>
                <p className="text-xs text-muted-foreground">
                  {previewPlatform === "twitter"
                    ? "@youraccount"
                    : "Your channel"}
                </p>
              </div>
            </div>

            <MoreHorizontal className="h-4 w-4 text-muted-foreground" />
          </div>

          <div className="space-y-4 p-4">
            <p className="min-h-16 whitespace-pre-wrap wrap-break-word text-sm leading-6">
              {caption || (
                <span className="text-muted-foreground">
                  Your post caption will appear here...
                </span>
              )}
            </p>

            {media.length > 0 ? (
              <div className="grid overflow-hidden rounded-lg">
                {media.slice(0, 4).map((item) => {
                  const isVideo = item.file.type.startsWith("video/");

                  return isVideo ? (
                    <video
                      key={item.id}
                      src={item.previewUrl}
                      controls
                      className="max-h-80 w-full object-cover"
                    />
                  ) : (
                    <img
                      key={item.id}
                      src={item.previewUrl}
                      alt=""
                      className="max-h-80 w-full object-cover"
                    />
                  );
                })}
              </div>
            ) : (
              <div className="flex h-32 items-center justify-center rounded-lg bg-muted/50">
                {previewPlatform === "youtube" ? (
                  <svg
                    role="img"
                    viewBox="0 0 24 24"
                    className="h-6 w-6 fill-current text-muted-foreground"
                    aria-label="YouTube"
                  >
                    <path d={siYoutube.path} />
                  </svg>
                ) : (
                  <ImageIcon className="h-6 w-6 text-muted-foreground" />
                )}
              </div>
            )}

            <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
              <span>Now</span>

              <span>
                {previewPlatform === "twitter"
                  ? "Post on X"
                  : "Upload to YouTube"}
              </span>
            </div>
          </div>
        </div>

        {platforms.length > 1 && (
          <p className="mt-4 text-center text-xs text-muted-foreground">
            Previewing {previewPlatform === "twitter" ? "X" : "YouTube"} ·{" "}
            {platforms.length} platforms selected
          </p>
        )}
      </CardContent>
    </Card>
  );
}
