"use client";

import { useAccounts } from "@/features/accounts/hooks/use-accounts";
import { createPost, retryPost } from "@/features/posts/api/create-post";
import { usePost } from "@/features/posts/hooks/use-posts";
import { deleteMedia, uploadMedia } from "@/features/posts/api/upload-media";
import type { ContentFormat, PostResponse } from "@/types/post";
import { CalendarClock, RefreshCcw, Send } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";

import { MediaUploader, type MediaFile } from "./media-uploader";
import { PlatformSelector, type Platform } from "./platform-selector";
import { PostPreview } from "./post-preview";
import { PublishingProgress } from "./publishing-progress";
import { ScheduleDialog } from "./schedule-dialog";
import { YouTubeFormatSelector } from "./youtube-format-selector";
import { VideoValidationCard } from "./video-validation-card";
import {
  extractVideoMetadata,
  validateVideoForFormat,
  type VideoMetadata,
  type VideoValidationResult,
} from "@/features/posts/utils/video-validator";

type SubmitPhase =
  | "idle"
  | "submitting" // HTTP request in flight (publish now)
  | "scheduling" // HTTP request in flight (schedule)
  | "scheduled" // backend accepted scheduled post
  | "queued" // backend accepted, job enqueued
  | "published" // terminal success
  | "failed"; // terminal failure

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
  const [youtubeFormat, setYoutubeFormat] = useState<ContentFormat>("VIDEO");

  // Video duration and aspect ratio validation state
  const [videoMetadata, setVideoMetadata] = useState<VideoMetadata | null>(null);
  const [videoValidation, setVideoValidation] = useState<VideoValidationResult | null>(null);
  const [isValidatingVideo, setIsValidatingVideo] = useState(false);
  const [validatingFileName, setValidatingFileName] = useState<string | undefined>(undefined);
  const activeValidationAbortRef = useRef<AbortController | null>(null);

  // Revalidate whenever youtubeFormat or videoMetadata changes
  useEffect(() => {
    if (videoMetadata) {
      const result = validateVideoForFormat(videoMetadata, youtubeFormat);
      setVideoValidation(result);
    }
  }, [youtubeFormat, videoMetadata]);

  // Submission state
  const [phase, setPhase] = useState<SubmitPhase>("idle");
  const [submittedPost, setSubmittedPost] = useState<PostResponse | null>(null);
  const [isRetrying, setIsRetrying] = useState(false);
  const [isScheduleSubmitting, setIsScheduleSubmitting] = useState(false);

  // Poll the post status after submission
  const isTracking =
    submittedPost !== null &&
    (phase === "queued" || phase === "published" || phase === "failed");
  const { data: polledPost } = usePost(
    isTracking ? submittedPost.id : null,
    isTracking,
  );

  // Sync phase with polled status
  const displayPost = polledPost ?? submittedPost;
  if (
    displayPost &&
    phase !== "idle" &&
    phase !== "submitting"
  ) {
    if (
      displayPost.status === "PUBLISHED" &&
      phase !== "published"
    ) {
      setPhase("published");
      toast.success("Post published successfully!");
    } else if (
      displayPost.status === "FAILED" &&
      phase !== "failed"
    ) {
      setPhase("failed");
      toast.error("Publishing failed. See details below.");
    }
  }

  const resolvedSelectedPlatforms = useMemo(() => {
    return selectedPlatforms.length > 0
      ? selectedPlatforms
      : connectedPlatforms.length > 0
        ? [connectedPlatforms[0]]
        : [];
  }, [selectedPlatforms, connectedPlatforms]);

  const isYouTubeSelected = resolvedSelectedPlatforms.includes("youtube");

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

  const handleUploadFile = useCallback(
    (localId: string, file: File) => {
      const isVideo = file.type.startsWith("video/");

      if (!isVideo) {
        // Non-video files upload directly
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
        return;
      }

      // Video file: extract metadata and validate locally in browser before uploading
      activeValidationAbortRef.current?.abort();
      const controller = new AbortController();
      activeValidationAbortRef.current = controller;

      // Reset previous validation and set validating state
      setVideoMetadata(null);
      setVideoValidation(null);
      setIsValidatingVideo(true);
      setValidatingFileName(file.name);

      setMedia((prev) =>
        prev.map((item) =>
          item.id === localId
            ? { ...item, uploadStatus: "idle", error: undefined }
            : item,
        ),
      );

      extractVideoMetadata(file, controller.signal)
        .then((meta) => {
          if (controller.signal.aborted) return;

          setIsValidatingVideo(false);
          setVideoMetadata(meta);

          const validationRes = validateVideoForFormat(meta, youtubeFormat);
          setVideoValidation(validationRes);

          // Mark upload as starting
          setMedia((prev) =>
            prev.map((item) =>
              item.id === localId
                ? { ...item, metadata: meta, uploadStatus: "uploading", uploadProgress: 0 }
                : item,
            ),
          );

          // Proceed with upload
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
        })
        .catch((err) => {
          if (controller.signal.aborted || err?.name === "AbortError") {
            return;
          }

          const errMsg =
            err instanceof Error
              ? err.message
              : "Failed to read video dimensions or duration.";

          setIsValidatingVideo(false);
          setVideoMetadata(null);
          setVideoValidation({
            status: "error",
            error: errMsg,
          });

          // Mark media item as error and PREVENT uploading
          setMedia((prev) =>
            prev.map((item) =>
              item.id === localId
                ? { ...item, uploadStatus: "error", error: errMsg }
                : item,
            ),
          );

          toast.error(`Video validation error: ${errMsg}`);
        });
    },
    [youtubeFormat],
  );

  const handleRemoveFile = useCallback((item: MediaFile) => {
    if (item.file.type.startsWith("video/")) {
      activeValidationAbortRef.current?.abort();
      setVideoMetadata(null);
      setVideoValidation(null);
      setIsValidatingVideo(false);
      setValidatingFileName(undefined);
    }
    if (item.mediaId) {
      deleteMedia(item.mediaId).catch((err) => {
        console.warn("Failed to delete unreferenced media asset:", err);
      });
    }
  }, []);

  const handlePublish = async () => {
    if (isLoadingAccounts) return;
    if (phase === "submitting") return; // prevent double submit

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
    if (isYouTubeSelected) {
      if (isValidatingVideo) {
        toast.error("Please wait for video metadata validation to finish before publishing.");
        return;
      }
      if (videoValidation?.status === "error") {
        toast.error(videoValidation.error || "Please resolve the video file error before publishing.");
        return;
      }
      const hasCompletedVideo = media.some(
        (m) =>
          m.file.type.startsWith("video/") &&
          m.uploadStatus === "complete" &&
          m.mediaId,
      );
      if (!hasCompletedVideo) {
        toast.error("YouTube Video/Short publishing requires a completed video upload.");
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
      toast.error("No connected account is available for the selected platforms.");
      return;
    }

    const completedMediaItems = media.filter(
      (m) => m.uploadStatus === "complete" && Boolean(m.mediaId),
    );
    const mediaIds = completedMediaItems
      .map((m) => m.mediaId)
      .filter((id): id is string => Boolean(id));
    const primaryMediaId = mediaIds.length > 0 ? mediaIds[0] : null;

    // Determine content_format:
    // Only send youtube-specific format if youtube is the sole/primary target.
    const contentFormat: ContentFormat = isYouTubeSelected
      ? youtubeFormat
      : "VIDEO";

    try {
      setPhase("submitting");

      const post = await createPost({
        caption: caption.trim(),
        targets,
        media_id: primaryMediaId,
        media_ids: mediaIds.length > 0 ? mediaIds : null,
        content_format: contentFormat,
      });

      setSubmittedPost(post);
      setPhase("queued");
      toast.success("Post accepted and queued for publishing.");
      // Do NOT reset the form here — keep data visible until success/failure is confirmed

    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to create the post.";
      toast.error(message);
      setPhase("idle");
    }
  };

  const handleRetry = async () => {
    if (!submittedPost || isRetrying) return;
    try {
      setIsRetrying(true);
      const updated = await retryPost(submittedPost.id);
      setSubmittedPost(updated);
      setPhase("queued");
      toast.success("Post requeued for publishing.");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Retry failed";
      toast.error(msg);
    } finally {
      setIsRetrying(false);
    }
  };

  const handleScheduleSubmit = async (isoDate: string) => {
    // Run same validations as handlePublish
    if (isLoadingAccounts) return;
    if (connectedAccounts.length === 0) {
      toast.error("Connect a social account before scheduling.");
      return;
    }
    if (!caption.trim()) {
      toast.error("Add a caption before scheduling.");
      return;
    }
    if (resolvedSelectedPlatforms.length === 0) {
      toast.error("Select at least one connected account.");
      return;
    }
    if (hasUploadingMedia) {
      toast.error("Please wait for all media uploads to finish before scheduling.");
      return;
    }
    const failedMedia = media.find((m) => m.uploadStatus === "error");
    if (failedMedia) {
      toast.error("Some media failed to upload. Remove or re-upload before scheduling.");
      return;
    }
    if (isYouTubeSelected) {
      if (isValidatingVideo) {
        toast.error("Please wait for video metadata validation to finish before scheduling.");
        return;
      }
      if (videoValidation?.status === "error") {
        toast.error(videoValidation.error || "Please resolve the video file error before scheduling.");
        return;
      }
      const hasCompletedVideo = media.some(
        (m) =>
          m.file.type.startsWith("video/") &&
          m.uploadStatus === "complete" &&
          m.mediaId,
      );
      if (!hasCompletedVideo) {
        toast.error("YouTube Video/Short scheduling requires a completed video upload.");
        return;
      }
    }

    const targets = resolvedSelectedPlatforms
      .map(
        (platform) =>
          connectedAccounts.find((account) => account.platform === platform)?.id,
      )
      .filter((target): target is string => Boolean(target));

    if (targets.length === 0) {
      toast.error("No connected account is available for the selected platforms.");
      return;
    }

    const completedMediaItems = media.filter(
      (m) => m.uploadStatus === "complete" && Boolean(m.mediaId),
    );
    const mediaIds = completedMediaItems
      .map((m) => m.mediaId)
      .filter((id): id is string => Boolean(id));
    const primaryMediaId = mediaIds.length > 0 ? mediaIds[0] : null;

    const contentFormat: ContentFormat = isYouTubeSelected ? youtubeFormat : "VIDEO";

    try {
      setScheduleOpen(false);
      setIsScheduleSubmitting(true);
      setPhase("scheduling");

      const post = await createPost({
        caption: caption.trim(),
        targets,
        media_id: primaryMediaId,
        media_ids: mediaIds.length > 0 ? mediaIds : null,
        content_format: contentFormat,
        scheduled_at: isoDate,
      });

      setSubmittedPost(post);
      setPhase("scheduled");
      toast.success("Post scheduled successfully!");
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Failed to schedule the post.";
      toast.error(message);
      setPhase("idle");
    } finally {
      setIsScheduleSubmitting(false);
    }
  };

  const handleReset = () => {
    // Only reset after a terminal state
    if (phase !== "published" && phase !== "failed" && phase !== "scheduled") return;
    activeValidationAbortRef.current?.abort();
    setVideoMetadata(null);
    setVideoValidation(null);
    setIsValidatingVideo(false);
    setValidatingFileName(undefined);
    media.forEach((item) => URL.revokeObjectURL(item.previewUrl));
    setCaption("");
    setSelectedPlatforms([]);
    setMedia([]);
    setYoutubeFormat("VIDEO");
    setPhase("idle");
    setSubmittedPost(null);
  };

  const isSubmitting = phase === "submitting";
  const isDisabled = isSubmitting || phase === "scheduling" || phase === "queued" || phase === "published" || phase === "scheduled";

  const publishButtonLabel = {
    idle: "Publish",
    submitting: "Submitting…",
    scheduling: "Scheduling…",
    scheduled: "Scheduled",
    queued: "Queued",
    published: "Published",
    failed: "Retry Publish",
  }[phase];

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

            {/* YouTube format selector — only when YouTube is selected */}
            {isYouTubeSelected && (
              <>
                <Separator />
                <YouTubeFormatSelector
                  value={youtubeFormat}
                  onChange={setYoutubeFormat}
                  disabled={isDisabled}
                />
              </>
            )}

            <Separator />

            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <label htmlFor="post-caption" className="text-sm font-medium">
                  {isYouTubeSelected ? "Title / Caption" : "Caption"}
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
                disabled={isDisabled}
                className="w-full resize-none rounded-lg border bg-background px-4 py-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20 disabled:cursor-not-allowed disabled:opacity-60"
              />
            </div>

            {/* Media uploader */}
            <MediaUploader
              media={media}
              onChange={setMedia}
              onUpload={handleUploadFile}
              onRemove={handleRemoveFile}
              disabled={isDisabled}
            />

            {/* Video duration & aspect ratio validation card */}
            {isYouTubeSelected && (isValidatingVideo || videoValidation) && (
              <VideoValidationCard
                format={youtubeFormat}
                validation={videoValidation}
                isValidating={isValidatingVideo}
                fileName={validatingFileName}
                onSwitchToNormalVideo={() => setYoutubeFormat("VIDEO")}
              />
            )}

            {/* Scheduled confirmation banner */}
            {phase === "scheduled" && submittedPost?.scheduled_at && (
              <div className="flex items-center gap-2 rounded-lg border border-primary/20 bg-primary/5 px-4 py-3 text-sm text-primary">
                <CalendarClock className="h-4 w-4 shrink-0" />
                <span>
                  Scheduled for{" "}
                  <strong>
                    {new Date(submittedPost.scheduled_at).toLocaleString(undefined, {
                      dateStyle: "medium",
                      timeStyle: "short",
                    })}
                  </strong>
                  {" — "}
                  <Link href="/calendar" className="underline underline-offset-2">
                    View calendar
                  </Link>
                </span>
              </div>
            )}

            {/* Publishing progress panel */}
            {displayPost && phase !== "idle" && phase !== "scheduled" && (
              <PublishingProgress
                post={displayPost}
                onRetry={phase === "failed" ? handleRetry : undefined}
                isRetrying={isRetrying}
              />
            )}
          </CardContent>

          <CardFooter className="flex flex-col gap-3 border-t bg-muted/30 px-6 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex gap-2">
              {(phase === "published" || phase === "failed" || phase === "scheduled") && (
                <Button
                  type="button"
                  variant="ghost"
                  onClick={handleReset}
                  className="w-full sm:w-auto"
                >
                  <RefreshCcw className="mr-2 h-4 w-4" />
                  New post
                </Button>
              )}
            </div>

            <div className="flex w-full gap-2 sm:w-auto">
              <Button
                type="button"
                variant="outline"
                onClick={() => setScheduleOpen(true)}
                className="flex-1 sm:flex-none"
                disabled={isDisabled}
              >
                <CalendarClock className="mr-2 h-4 w-4" />
                Schedule
              </Button>

              <Button
                type="button"
                onClick={phase === "failed" ? handleRetry : handlePublish}
                disabled={
                  isDisabled ||
                  isRetrying ||
                  hasUploadingMedia ||
                  !caption.trim() ||
                  charactersRemaining < 0 ||
                  connectedAccounts.length === 0
                }
                className="flex-1 sm:flex-none"
              >
                <Send className="mr-2 h-4 w-4" />
                {publishButtonLabel}
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
        onSchedule={handleScheduleSubmit}
        isSubmitting={isScheduleSubmitting}
      />
    </>
  );
}
