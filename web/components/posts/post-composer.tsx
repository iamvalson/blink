"use client";

import { useAccounts } from "@/features/accounts/hooks/use-accounts";
import { createPost } from "@/features/posts/api/create-post";
import { uploadMedia } from "@/features/posts/api/upload-media";
import { CalendarClock, Save, Send } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

import { MediaUploader, type MediaFile } from "./media-uploader";
import { PlatformSelector, type Platform } from "./platform-selector";
import { PostPreview } from "./post-preview";
import { ScheduleDialog } from "./schedule-dialog";

export function PostComposer() {
  const { data, isLoading: isLoadingAccounts } = useAccounts();
  const connectedAccounts = data?.accounts ?? [];
  const connectedPlatforms = useMemo(
    () =>
      Array.from(
        new Set(
          (data?.accounts ?? [])
            .map((account) => account.platform)
            .filter(
              (platform): platform is Platform =>
                platform === "twitter" || platform === "youtube",
            ),
        ),
      ),
    [data?.accounts],
  );

  const [caption, setCaption] = useState("");
  const [selectedPlatforms, setSelectedPlatforms] = useState<Platform[]>([]);
  const [media, setMedia] = useState<MediaFile[]>([]);
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const resolvedSelectedPlatforms =
    selectedPlatforms.length > 0
      ? selectedPlatforms
      : connectedPlatforms.length > 0
        ? [connectedPlatforms[0]]
        : [];

  const characterLimit = useMemo(() => {
    if (resolvedSelectedPlatforms.includes("twitter")) {
      return 280;
    }

    return 5000;
  }, [resolvedSelectedPlatforms]);

  const charactersRemaining = characterLimit - caption.length;

  const hasUploadingMedia = useMemo(() => {
    return media.some((m) => m.uploadStatus === "uploading");
  }, [media]);

  const togglePlatform = (platform: Platform) => {
    if (!connectedPlatforms.includes(platform)) {
      return;
    }

    setSelectedPlatforms((current) => {
      if (current.includes(platform)) {
        if (current.length === 1) {
          return current;
        }

        return current.filter((item) => item !== platform);
      }

      return [...current, platform];
    });
  };

  const handleUploadFile = useCallback((localId: string, file: File) => {
    setMedia((prev) =>
      prev.map((item) =>
        item.id === localId
          ? { ...item, uploadStatus: "uploading", uploadProgress: 0, error: undefined }
          : item,
      ),
    );

    uploadMedia(file, (percent) => {
      setMedia((prev) =>
        prev.map((item) =>
          item.id === localId ? { ...item, uploadProgress: percent } : item,
        ),
      );
    })
      .then((res) => {
        setMedia((prev) =>
          prev.map((item) =>
            item.id === localId
              ? {
                  ...item,
                  uploadStatus: "complete",
                  uploadProgress: 100,
                  mediaId: res.id,
                }
              : item,
          ),
        );
      })
      .catch((err) => {
        const message = err instanceof Error ? err.message : "Upload failed";
        setMedia((prev) =>
          prev.map((item) =>
            item.id === localId
              ? { ...item, uploadStatus: "error", error: message }
              : item,
          ),
        );
        toast.error(`Media upload failed: ${message}`);
      });
  }, []);

  const handlePublish = async () => {
    if (isLoadingAccounts) {
      return;
    }

    if (connectedAccounts.length === 0) {
      toast.error("Connect a social account before publishing.");
      return;
    }

    if (!caption.trim()) {
      toast.error("Add a caption before publishing.");
      return;
    }

    if (resolvedSelectedPlatforms.length === 0) {
      toast.error("Select at least one connected account to publish to.");
      return;
    }

    if (hasUploadingMedia) {
      toast.error("Please wait for all media uploads to finish before publishing.");
      return;
    }

    const failedMedia = media.find((m) => m.uploadStatus === "error");
    if (failedMedia) {
      toast.error("Some media failed to upload. Remove or re-upload before publishing.");
      return;
    }

    // Platform-specific media validations
    if (resolvedSelectedPlatforms.includes("youtube")) {
      const hasCompletedVideo = media.some(
        (m) =>
          m.file.type.startsWith("video/") &&
          m.uploadStatus === "complete" &&
          m.mediaId,
      );
      if (!hasCompletedVideo) {
        toast.error("YouTube publishing requires a completed video upload.");
        return;
      }
    }

    const targets = resolvedSelectedPlatforms
      .map(
        (platform) =>
          connectedAccounts.find((account) => account.platform === platform)
            ?.id,
      )
      .filter((target): target is string => Boolean(target));

    if (targets.length === 0) {
      toast.error(
        "No connected account is available for the selected platforms.",
      );
      return;
    }

    // Primary attached media
    const completedMedia = media.find((m) => m.uploadStatus === "complete" && m.mediaId);
    const mediaId = completedMedia?.mediaId ?? null;

    try {
      setIsSubmitting(true);
      const post = await createPost({
        caption: caption.trim(),
        targets,
        media_id: mediaId,
      });

      toast.success(`Post queued for publishing (${post.status}).`);
      media.forEach((item) => URL.revokeObjectURL(item.previewUrl));
      setCaption("");
      setSelectedPlatforms([]);
      setMedia([]);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create the post.";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleSaveDraft = () => {
    // API integration comes next.
    console.log("Saving draft", {
      caption,
      platforms: selectedPlatforms,
      media,
    });
  };

  return (
    <>
      <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_380px]">
        <Card className="overflow-hidden">
          <CardContent className="space-y-6 p-6">
            <div>
              <h2 className="text-sm font-medium">Publish to</h2>
              <p className="mt-1 text-sm text-muted-foreground">
                Choose where you want this post to appear.
              </p>

              {connectedAccounts.length === 0 ? (
                <div className="mt-4 rounded-lg border border-dashed bg-muted/30 p-4 text-sm text-muted-foreground">
                  <p className="font-medium text-foreground">
                    No connected accounts yet.
                  </p>
                  <p className="mt-1">
                    Connect a social account before publishing.
                  </p>
                  <Link
                    href="/accounts"
                    className="mt-3 inline-flex items-center text-sm font-medium text-primary hover:underline"
                  >
                    Manage accounts
                  </Link>
                </div>
              ) : (
                <div className="mt-4">
                  <PlatformSelector
                    selectedPlatforms={resolvedSelectedPlatforms}
                    onToggle={togglePlatform}
                    connectedPlatforms={connectedPlatforms}
                  />
                </div>
              )}
            </div>

            <Separator />

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <label htmlFor="post-caption" className="text-sm font-medium">
                  Caption
                </label>

                <span
                  className={`text-xs ${
                    charactersRemaining < 0
                      ? "text-destructive"
                      : "text-muted-foreground"
                  }`}
                >
                  {charactersRemaining} characters left
                </span>
              </div>

              <textarea
                id="post-caption"
                value={caption}
                onChange={(event) => setCaption(event.target.value)}
                placeholder="What do you want to share?"
                maxLength={characterLimit}
                rows={9}
                className="w-full resize-none rounded-lg border bg-background px-4 py-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20"
              />
            </div>

            <MediaUploader
              media={media}
              onChange={setMedia}
              onUpload={handleUploadFile}
              disabled={isSubmitting}
            />
          </CardContent>

          <CardFooter className="flex flex-col gap-3 border-t bg-muted/30 px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
            <Button
              type="button"
              variant="ghost"
              onClick={handleSaveDraft}
              className="w-full sm:w-auto"
            >
              <Save className="mr-2 h-4 w-4" />
              Save draft
            </Button>

            <div className="flex w-full gap-2 sm:w-auto">
              <Button
                type="button"
                variant="outline"
                onClick={() => setScheduleOpen(true)}
                className="flex-1 sm:flex-none"
              >
                <CalendarClock className="mr-2 h-4 w-4" />
                Schedule
              </Button>

              <Button
                type="button"
                onClick={handlePublish}
                disabled={
                  isSubmitting ||
                  hasUploadingMedia ||
                  !caption.trim() ||
                  charactersRemaining < 0 ||
                  connectedAccounts.length === 0
                }
                className="flex-1 sm:flex-none"
              >
                <Send className="mr-2 h-4 w-4" />
                {isSubmitting ? "Publishing..." : "Publish"}
              </Button>
            </div>
          </CardFooter>
        </Card>

        <div className="lg:sticky lg:top-6 lg:self-start">
          <PostPreview
            caption={caption}
            media={media}
            platforms={resolvedSelectedPlatforms}
          />
        </div>
      </div>

      <ScheduleDialog
        open={scheduleOpen}
        onOpenChange={setScheduleOpen}
        onSchedule={(date) => {
          console.log("Schedule post:", {
            caption,
            platforms: resolvedSelectedPlatforms,
            media,
            date,
          });

          setScheduleOpen(false);
        }}
      />
    </>
  );
}
