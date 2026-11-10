"use client";

import { useQuery } from "@tanstack/react-query";
import { getTikTokCreatorInfo } from "@/features/posts/api/tiktok-creator-info";
import { emptyTikTokOptions, effectiveTikTokOptions, validateTikTokPost, type TikTokOptions } from "@/features/posts/utils/tiktok-validator";
import type { InstagramFormat } from "@/types/post";
import { TikTokSettings } from "./tiktok-settings";
import { useAccounts } from "@/features/accounts/hooks/use-accounts";
import { createPost, retryPost } from "@/features/posts/api/create-post";
import { usePost } from "@/features/posts/hooks/use-posts";
import { deleteMedia, uploadMedia } from "@/features/posts/api/upload-media";
import type { ContentFormat, PostResponse } from "@/types/post";
import { CalendarClock, Eye, Pencil, RefreshCcw, Send } from "lucide-react";
import Link from "next/link";
import { useCallback, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Separator } from "@/components/ui/separator";

import { InstagramPreparedPhoto, useInstagramMedia } from "./instagram-feed-photo";
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
import {
  validateXMediaConstraints,
  xMediaKind,
} from "@/features/posts/utils/x-media-validator";

const captionSettings: Record<Platform, { name: string; limit: number }> = {
  twitter: { name: "X", limit: 280 },
  instagram: { name: "Instagram", limit: 2200 },
  tiktok: { name: "TikTok", limit: 2200 },
  youtube: { name: "YouTube", limit: 5000 },
};

function captionError(platform: Platform, caption: string) {
  const { name, limit } = captionSettings[platform];
  if (!caption.trim() && platform !== "instagram") return `Add a caption for ${name}.`;
  if (caption.trim().length > limit) return `${name} captions can contain up to ${limit} characters.`;
  return null;
}

type SubmitPhase =
  | "idle"
  | "submitting" // HTTP request in flight (publish now)
  | "scheduling" // HTTP request in flight (schedule)
  | "scheduled" // backend accepted scheduled post
  | "queued" // backend accepted, job enqueued
  | "published" // terminal success
  | "partial"
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
                platform === "twitter" || platform === "youtube" || platform === "tiktok" || platform === "instagram",
            ),
        ),
      ),
    [data?.accounts],
  );

  const [composerView, setComposerView] = useState<"write" | "preview">("write");
  const [caption, setCaption] = useState("");
  const [captionOverrides, setCaptionOverrides] = useState<Partial<Record<Platform, string>>>({});
  const [captionEditor, setCaptionEditor] = useState<{ platform: Platform; text: string } | null>(null);
  const [previewPlatform, setPreviewPlatform] = useState<Platform | null>(null);
  const [tikTokInboxAccount, setTikTokInboxAccount] = useState("");
  const [tikTokInboxConsentMedia, setTikTokInboxConsentMedia] = useState<string | null>(null);
  const [tikTokAccount, setTikTokAccount] = useState("");
  const [tikTokChoices, setTikTokChoices] = useState<Record<string,TikTokOptions>>({});
  const [selectedPlatforms, setSelectedPlatforms] = useState<Platform[]>([]);
  const [instagramFormat, setInstagramFormat] = useState<InstagramFormat>("IMAGE");
  const [media, setMedia] = useState<MediaFile[]>([]);
  const [scheduleOpen, setScheduleOpen] = useState(false);
  const [youtubeFormat, setYoutubeFormat] = useState<ContentFormat>("VIDEO");

  // Video duration and aspect ratio validation state
  const [videoMetadata, setVideoMetadata] = useState<VideoMetadata | null>(null);
  const [videoValidationState, setVideoValidation] = useState<VideoValidationResult | null>(null);
  const [isValidatingVideo, setIsValidatingVideo] = useState(false);
  const [validatingFileName, setValidatingFileName] = useState<string | undefined>(undefined);
  const videoValidations = useRef(new Map<string, AbortController>());
  const latestVideoValidation = useRef<string | null>(null);

  const videoValidation = useMemo(() => videoMetadata ? validateVideoForFormat(videoMetadata,youtubeFormat) : videoValidationState,[videoMetadata,youtubeFormat,videoValidationState]);

  // Submission state
  const [phase, setPhase] = useState<SubmitPhase>("idle");
  const [submittedPost, setSubmittedPost] = useState<PostResponse | null>(null);
  const [isRetrying, setIsRetrying] = useState(false);
  const [isScheduleSubmitting, setIsScheduleSubmitting] = useState(false);

  // Poll the post status after submission
  const isTracking =
    submittedPost !== null &&
    (phase === "queued" || phase === "published" || phase === "failed" || phase === "partial");
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
      ["PUBLISHED", "INBOX_DELIVERED"].includes(displayPost.status) &&
      phase !== "published"
    ) {
      setPhase("published");
      toast.success(displayPost.status === "INBOX_DELIVERED" ? "Video sent to your TikTok Inbox." : "Post published successfully!");
    } else if (displayPost.status === "PARTIALLY_PUBLISHED" && phase !== "partial") {
      setPhase("partial");
      toast.warning("Some platforms published. Review failed targets before retrying.");
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

  const isTikTokSelected = resolvedSelectedPlatforms.includes("tiktok");
  const tikTokAccounts = connectedAccounts.filter(a => a.platform === "tiktok");
  const tikTokAccountID = tikTokAccounts.some(a => a.id === tikTokAccount) ? tikTokAccount : (tikTokAccounts[0]?.id ?? "");
  const canSendToTikTokInbox = isTikTokSelected && resolvedSelectedPlatforms.length === 1 && Boolean(tikTokAccountID);
  const sendToTikTokInbox = canSendToTikTokInbox && tikTokInboxAccount === tikTokAccountID;
  const selectedTargets = resolvedSelectedPlatforms.flatMap((platform) => {
    const account = connectedAccounts.find((account) =>
      platform === "tiktok" ? account.id === tikTokAccountID : account.platform === platform,
    );
    return account ? [{ platform, id: account.id }] : [];
  });
  const targets = selectedTargets.map(({ id }) => id);
  const instagramTarget = selectedTargets.find(target => target.platform === "instagram");
  const isInstagramReel = Boolean(instagramTarget && instagramFormat === "REELS");
  const instagramFormats = instagramTarget ? { [instagramTarget.id]: instagramFormat } : undefined;
  const instagramInputError = instagramTarget && validateInstagramMedia(
    instagramFormat, media.map(item => item.file.type), resolvedSelectedPlatforms.length,
  );
  const instagramPreparation = useInstagramMedia(media, instagramFormat, Boolean(instagramTarget && !instagramInputError));
  const uploadedInstagramPhotos = useRef(new WeakMap<File, string>());
  const instagramError = instagramInputError ?? instagramPreparation.error;

  async function uploadInstagramAttachments(originalIds: string[]) {
    const items = instagramPreparation.items;
    if (!items?.some(item => item.photo)) return undefined;
    const ids = [...originalIds];
    for (const item of items) {
      const file = item.photo?.file;
      if (!file) continue;
      let id = uploadedInstagramPhotos.current.get(file);
      if (!id) {
        try {
          ({ id } = await uploadMedia(file));
        } catch (error) {
          throw new Error(`Instagram item ${item.index + 1}: ${error instanceof Error ? error.message : "upload failed"}`);
        }
        uploadedInstagramPhotos.current.set(file, id);
      }
      ids[item.index] = id;
    }
    return ids;
  }
  const captionOverridesPayload: Record<string, string> = {};
  for (const { id, platform } of selectedTargets) {
    const override = captionOverrides[platform];
    if (!sendToTikTokInbox && override !== undefined) captionOverridesPayload[id] = override.trim();
  }
  const captionErrors = sendToTikTokInbox ? [] : selectedTargets.flatMap(({ platform }) => {
    const error = platform === "instagram" && instagramFormat === "STORIES" ? null : captionError(platform, captionOverrides[platform] ?? caption);
    return error ? [{ platform, error }] : [];
  });
  const activePreview = previewPlatform && resolvedSelectedPlatforms.includes(previewPlatform)
    ? previewPlatform
    : resolvedSelectedPlatforms[0];

  const creatorQuery = useQuery({queryKey:["tiktok-creator",tikTokAccountID],queryFn:()=>getTikTokCreatorInfo(tikTokAccountID),enabled:isTikTokSelected && Boolean(tikTokAccountID) && !sendToTikTokInbox,staleTime:0,refetchOnMount:"always",retry:false});
  const tikTokOptions = effectiveTikTokOptions(
    { ...emptyTikTokOptions, ...tikTokChoices[tikTokAccountID], duration_seconds: videoMetadata?.duration ?? 0 },
    creatorQuery.data,
  );
  const inboxMediaID = media.length === 1 ? media[0].mediaId : null;
  const inboxOptions = {
    ...emptyTikTokOptions,
    send_to_inbox: true,
    consent: Boolean(inboxMediaID && inboxMediaID === tikTokInboxConsentMedia),
  };
  const selectedTikTokOptions = sendToTikTokInbox ? inboxOptions : tikTokOptions;
  const tikTokMedia = media.map(item => ({
    contentType: item.file.type,
    complete: item.uploadStatus === "complete" && Boolean(item.mediaId),
    size: item.file.size,
  }));
  let tikTokError: string | null = null;
  if (isTikTokSelected) {
    if (!sendToTikTokInbox && creatorQuery.isFetching) {
      tikTokError = "Loading current TikTok creator settings…";
    } else if (!sendToTikTokInbox && creatorQuery.isError) {
      tikTokError = "TikTok creator settings could not be loaded. Refresh to try again.";
    } else {
      tikTokError = validateTikTokPost(selectedTikTokOptions, creatorQuery.data, tikTokMedia, videoMetadata, (captionOverrides.tiktok ?? caption).trim());
    }
  }
  const tikTokPayload = isTikTokSelected ? { [tikTokAccountID]: selectedTikTokOptions } : undefined;
  const isYouTubeSelected = resolvedSelectedPlatforms.includes("youtube");
  const isXSelected = resolvedSelectedPlatforms.includes("twitter");

  // X (Twitter) media constraints — compute live from current media list
  const xMediaError = useMemo(() => {
    if (!isXSelected || media.length === 0) return null;
    return validateXMediaConstraints(media.map((m) => m.file.type));
  }, [isXSelected, media]);

  // Compute whether the "Add more media" button should be blocked when X is selected.
  const xAddBlockedReason = useMemo((): string | null => {
    if (!isXSelected || media.length === 0) return null;
    const kinds = media.map((m) => xMediaKind(m.file.type));
    const hasVideo = kinds.includes("video");
    const hasGif = kinds.includes("gif");
    const hasPhoto = kinds.includes("photo");
    if (hasVideo) return "X allows at most 1 video per post. Remove the video to add different media.";
    if (hasGif) return "X allows at most 1 animated GIF per post. Remove the GIF to add different media.";
    if (hasPhoto && media.length >= 4) return "X allows at most 4 photos per post.";
    return null;
  }, [isXSelected, media]);

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
      const controller = new AbortController();
      videoValidations.current.set(localId, controller);
      latestVideoValidation.current = localId;

      // Reset previous validation and set validating state
      setVideoMetadata(null);
      setVideoValidation(null);
      setIsValidatingVideo(true);
      setValidatingFileName(file.name);

      setMedia((prev) =>
        prev.map((item) =>
          item.id === localId
            ? { ...item, uploadStatus: "uploading", uploadProgress: 0, error: undefined }
            : item,
        ),
      );

      extractVideoMetadata(file, controller.signal)
        .then((meta) => {
          if (controller.signal.aborted) return;

          if (latestVideoValidation.current === localId) {
            setIsValidatingVideo(false);
            setVideoMetadata(meta);
            setVideoValidation(validateVideoForFormat(meta, youtubeFormat));
          }

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

          if (latestVideoValidation.current === localId) {
            setIsValidatingVideo(false);
            setVideoMetadata(null);
            setVideoValidation({ status: "error", error: errMsg });
          }

          // Mark media item as error and PREVENT uploading
          setMedia((prev) =>
            prev.map((item) =>
              item.id === localId
                ? { ...item, uploadStatus: "error", error: errMsg }
                : item,
            ),
          );

          toast.error(`Video validation error: ${errMsg}`);
        })
        .finally(() => videoValidations.current.delete(localId));
    },
    [youtubeFormat],
  );

  const handleRemoveFile = useCallback((item: MediaFile) => {
    videoValidations.current.get(item.id)?.abort();
    if (latestVideoValidation.current === item.id) {
      latestVideoValidation.current = null;
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
    if (tikTokError) { toast.error(tikTokError); return; }
    if (instagramError) { toast.error(instagramError); return; }
    if (phase === "submitting") return; // prevent double submit

    if (connectedAccounts.length === 0) {
      toast.error("Connect a social account before publishing.");
      return;
    }

    if (captionErrors.length > 0) {
      toast.error(captionErrors[0].error);
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

    // X (Twitter) platform-specific media constraint check
    if (isXSelected && xMediaError) {
      toast.error(`X media: ${xMediaError.message}`);
      return;
    }

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

    // Determine content_format:    // Only send youtube-specific format if youtube is the sole/primary target.
    const contentFormat: ContentFormat = isYouTubeSelected
      ? youtubeFormat
      : "VIDEO";

    try {
      setPhase("submitting");

      const post = await createPost({
        caption: sendToTikTokInbox ? "" : caption.trim(),
        targets,
        caption_overrides: Object.keys(captionOverridesPayload).length ? captionOverridesPayload : undefined,
        media_id: primaryMediaId,
        media_ids: mediaIds.length > 0 ? mediaIds : null,
        content_format: contentFormat,
        tiktok_options: tikTokPayload,
        instagram_formats: instagramFormats,
        instagram_media_ids: await uploadInstagramAttachments(mediaIds),
      });

      setSubmittedPost(post);
      setPhase("queued");
      toast.success(sendToTikTokInbox ? "Video queued for TikTok Inbox delivery." : "Post accepted and queued for publishing.");
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
    if (sendToTikTokInbox) return;
    // Run same validations as handlePublish
    if (isLoadingAccounts) return;
    if (tikTokError) { toast.error(tikTokError); return; }
    if (instagramError) { toast.error(instagramError); return; }
    if (connectedAccounts.length === 0) {
      toast.error("Connect a social account before scheduling.");
      return;
    }
    if (captionErrors.length > 0) {
      toast.error(captionErrors[0].error);
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

    // X (Twitter) platform-specific media constraint check
    if (isXSelected && xMediaError) {
      toast.error(`X media: ${xMediaError.message}`);
      return;
    }

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
        caption_overrides: Object.keys(captionOverridesPayload).length ? captionOverridesPayload : undefined,
        media_id: primaryMediaId,
        media_ids: mediaIds.length > 0 ? mediaIds : null,
        content_format: contentFormat,
        tiktok_options: tikTokPayload,
        instagram_formats: instagramFormats,
        instagram_media_ids: await uploadInstagramAttachments(mediaIds),
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
    if (phase !== "published" && phase !== "failed" && phase !== "partial" && phase !== "scheduled") return;
    for (const controller of videoValidations.current.values()) controller.abort();
    videoValidations.current.clear();
    latestVideoValidation.current = null;
    setVideoMetadata(null);
    setVideoValidation(null);
    setIsValidatingVideo(false);
    setValidatingFileName(undefined);
    media.forEach((item) => URL.revokeObjectURL(item.previewUrl));
    setCaption("");
    setCaptionOverrides({});
    setPreviewPlatform(null);
    setSelectedPlatforms([]);
    setMedia([]);
    setYoutubeFormat("VIDEO");
    setInstagramFormat("IMAGE");
    uploadedInstagramPhotos.current = new WeakMap();
    setTikTokChoices({});
    setTikTokInboxAccount("");
    setTikTokInboxConsentMedia(null);
    setPhase("idle");
    setSubmittedPost(null);
  };

  const isSubmitting = phase === "submitting";
  const isDisabled = isSubmitting || phase === "scheduling" || phase === "queued" || phase === "published" || phase === "scheduled";

  const publishButtonLabel = {
    idle: sendToTikTokInbox ? "Send to TikTok Inbox" : "Publish",
    submitting: "Submitting…",
    scheduling: "Scheduling…",
    scheduled: "Scheduled",
    queued: "Queued",
    published: displayPost?.status === "INBOX_DELIVERED" ? "Sent to TikTok Inbox" : "Published",
    failed: sendToTikTokInbox ? "Retry Inbox upload" : "Retry Publish",
    partial: "Retry failed targets",
  }[phase];

  return (
    <>
      <div className="mb-4 grid grid-cols-2 gap-1 rounded-xl bg-muted p-1 xl:hidden" role="group" aria-label="Composer view">
        <button
          type="button"
          aria-controls="composer-write"
          aria-pressed={composerView === "write"}
          onClick={() => setComposerView("write")}
          className="flex min-h-11 items-center justify-center gap-2 rounded-lg text-sm font-medium text-muted-foreground aria-pressed:bg-background aria-pressed:text-foreground aria-pressed:shadow-sm focus-visible:outline-2"
        >
          <Pencil className="size-4" aria-hidden="true" />
          Write
        </button>
        <button
          type="button"
          aria-controls="composer-preview"
          aria-pressed={composerView === "preview"}
          onClick={() => setComposerView("preview")}
          className="flex min-h-11 items-center justify-center gap-2 rounded-lg text-sm font-medium text-muted-foreground aria-pressed:bg-background aria-pressed:text-foreground aria-pressed:shadow-sm focus-visible:outline-2"
        >
          <Eye className="size-4" aria-hidden="true" />
          Preview
        </button>
      </div>
      <div className="grid grid-cols-1 items-start gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,380px)]">
        <Card id="composer-write" className={composerView === "preview" ? "hidden min-w-0 overflow-hidden xl:flex" : "min-w-0 overflow-hidden"}>
          <CardContent className="space-y-6 p-4 sm:p-6">
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

            {instagramTarget && (
              <div className="space-y-2">
                <label className="block text-sm font-medium">Instagram format
                  <select
                    value={instagramFormat}
                    disabled={isDisabled}
                    onChange={(event) => setInstagramFormat(event.target.value as InstagramFormat)}
                    className="mt-1 block min-h-11 w-full rounded border bg-background p-2"
                  >
                    <option value="IMAGE">Feed photo</option>
                    <option value="REELS">Reel</option>
                    <option value="CAROUSEL">Carousel</option>
                    <option value="STORIES">Story (eligible accounts)</option>
                  </select>
                </label>
                {instagramFormat === "CAROUSEL" && <p className="text-sm text-muted-foreground">Carousel photos use a square frame. Place a photo first to keep that frame when mixing photos and videos.</p>}
                {instagramFormat === "STORIES" && <p className="text-sm text-muted-foreground">Instagram Stories use the media without a caption.</p>}
                {instagramPreparation.items?.map(item => <InstagramPreparedPhoto key={item.id} item={item} disabled={isDisabled} />)}
                {instagramError && <p className="text-sm text-muted-foreground" role="status">{instagramError}</p>}
              </div>
            )}

            {isTikTokSelected && <>
              <label className="block text-sm font-medium">TikTok account
                <select value={tikTokAccountID} onChange={event => {
                  setTikTokAccount(event.target.value);
                  setTikTokInboxAccount("");
                  setTikTokInboxConsentMedia(null);
                }} disabled={isDisabled} className="mt-1 block min-h-11 w-full min-w-0 rounded border bg-background p-2 text-base sm:text-sm">
                  {tikTokAccounts.map(a=><option key={a.id} value={a.id}>{a.platform_user_id}</option>)}
                </select>
              </label>
              {canSendToTikTokInbox && (
                <label className="block text-sm font-medium">
                  TikTok delivery
                  <select
                    value={sendToTikTokInbox ? "inbox" : "direct"}
                    onChange={event => {
                      setTikTokInboxAccount(event.target.value === "inbox" ? tikTokAccountID : "");
                      setTikTokInboxConsentMedia(null);
                    }}
                    disabled={isDisabled || phase !== "idle"}
                    className="mt-1 block min-h-11 w-full rounded border bg-background p-2"
                  >
                    <option value="direct">Publish directly</option>
                    <option value="inbox">Send as draft to TikTok Inbox</option>
                  </select>
                </label>
              )}
              {sendToTikTokInbox ? (
                <fieldset disabled={isDisabled || phase !== "idle"} className="space-y-2 rounded-lg border p-3 text-sm">
                  <legend className="px-1 font-medium">TikTok Inbox upload</legend>
                  <p>After delivery, open the notification in your TikTok Inbox to edit the video, add a caption, and post in TikTok.</p>
                  <label className="flex items-start gap-2">
                    <input
                      type="checkbox"
                      checked={inboxOptions.consent}
                      disabled={!inboxMediaID}
                      onChange={event => setTikTokInboxConsentMedia(event.target.checked ? inboxMediaID ?? null : null)}
                    />
                    I authorize Blink to send this video to my TikTok Inbox.
                  </label>
                  {tikTokError && <p className="text-destructive">{tikTokError}</p>}
                </fieldset>
              ) : <TikTokSettings key={tikTokAccountID} creator={creatorQuery.data} options={tikTokOptions} onChange={options=>setTikTokChoices(previous=>({...previous,[tikTokAccountID]:options}))} disabled={isDisabled} loading={creatorQuery.isFetching} error={tikTokError} onRefresh={()=>{void creatorQuery.refetch();}} />}
            </>}
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

            {!sendToTikTokInbox && <div className="space-y-3">
              <label htmlFor="post-caption" className="text-sm font-medium">Shared caption</label>
              <p className="text-xs text-muted-foreground">
                Used by every selected platform that follows the shared caption.
              </p>

              <textarea
                id="post-caption"
                value={caption}
                onChange={(event) => setCaption(event.target.value)}
                placeholder="What do you want to share?"
                rows={9}
                disabled={isDisabled}
                className="w-full resize-none rounded-lg border bg-background px-3 py-3 text-base outline-none sm:px-4 sm:text-sm transition-colors placeholder:text-muted-foreground focus:border-ring focus:ring-2 focus:ring-ring/20 disabled:cursor-not-allowed disabled:opacity-60"
              />
              {selectedTargets.map(({ platform }) => {
                const { name, limit } = captionSettings[platform];
                const override = captionOverrides[platform];
                const count = (override ?? caption).trim().length;
                const error = captionErrors.find((entry) => entry.platform === platform)?.error;

                return (
                  <div key={platform} className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3">
                    <div className="min-w-0 space-y-1">
                      <p className="text-sm font-medium">{name}</p>
                      <p className="text-xs text-muted-foreground">
                        {override === undefined ? "Uses shared caption" : "Custom caption"} · {count}/{limit}
                      </p>
                      {count > 0 && error && <p className="text-xs text-destructive">{error}</p>}
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      aria-label={`Edit caption for ${name}`}
                      disabled={isDisabled}
                      onClick={() => {
                        setCaptionEditor({ platform, text: override ?? caption });
                        setPreviewPlatform(platform);
                      }}
                    >
                      Edit caption
                    </Button>
                  </div>
                );
              })}
            </div>}

            {/* Media uploader */}
            <MediaUploader
              media={media}
              onChange={setMedia}
              onUpload={handleUploadFile}
              onRemove={handleRemoveFile}
              disabled={isDisabled}
              maxFiles={isInstagramReel ? 1 : undefined}
              accept={isInstagramReel ? "video/mp4,video/quicktime,.mp4,.mov" : undefined}
              fileHint={isInstagramReel ? "One MP4 or MOV video" : undefined}
              addBlockedReason={isInstagramReel && media.length > 0
                ? (media.length > 1 ? "Remove extra attachments to use one video for this Reel." : "Instagram Reels use one video. Remove it to add a replacement.")
                : isXSelected ? xAddBlockedReason : null}
            />

            {/* X (Twitter) media constraint violation inline card */}
            {isXSelected && xMediaError && !isDisabled && (
              <div className="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-4 py-3 text-xs text-destructive">
                <svg xmlns="http://www.w3.org/2000/svg" className="mt-0.5 h-4 w-4 shrink-0" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} aria-hidden>
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
                </svg>
                <span>{xMediaError.message} Remove conflicting files to continue.</span>
              </div>
            )}

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
                onRetry={(phase === "failed" || phase === "partial") ? handleRetry : undefined}
                isRetrying={isRetrying}
              />
            )}
          </CardContent>

          <CardFooter className="flex flex-wrap gap-3 border-t bg-muted/30 px-4 py-4 sm:px-6">
            {(phase === "published" || phase === "failed" || phase === "partial" || phase === "scheduled") && (
              <Button
                type="button"
                variant="ghost"
                onClick={handleReset}
                className="min-h-11 w-full sm:w-auto"
              >
                <RefreshCcw className="mr-2 h-4 w-4" />
                New post
              </Button>
            )}

            <div className="flex w-full flex-wrap gap-2 sm:ml-auto sm:w-auto">
              <Button
                type="button"
                variant="outline"
                onClick={() => setScheduleOpen(true)}
                className="min-h-11 flex-1 sm:flex-none"
                disabled={isDisabled || sendToTikTokInbox || Boolean(tikTokError || instagramError) || captionErrors.length > 0}
              >
                <CalendarClock className="mr-2 h-4 w-4" />
                Schedule
              </Button>

              <Button
                type="button"
                onClick={(phase === "failed" || phase === "partial") ? handleRetry : handlePublish}
                disabled={
                  isDisabled ||
                  isRetrying ||
                  (phase !== "failed" && phase !== "partial" && (Boolean(tikTokError || instagramError) || captionErrors.length > 0)) ||
                  hasUploadingMedia ||
                  connectedAccounts.length === 0
                }
                className="min-h-11 flex-1 sm:flex-none"
              >
                <Send className="mr-2 h-4 w-4" />
                {publishButtonLabel}
              </Button>
            </div>
          </CardFooter>
        </Card>

        <div id="composer-preview" className={composerView === "write" ? "hidden min-w-0 xl:sticky xl:top-20 xl:block" : "min-w-0 xl:sticky xl:top-20"}>
          {resolvedSelectedPlatforms.length > 1 && (
            <div role="group" aria-label="Preview platform" className="mb-3 flex flex-wrap gap-2">
              {resolvedSelectedPlatforms.map((platform) => (
                <Button
                  key={platform}
                  type="button"
                  size="sm"
                  variant={activePreview === platform ? "default" : "outline"}
                  aria-pressed={activePreview === platform}
                  onClick={() => setPreviewPlatform(platform)}
                >
                  {captionSettings[platform].name}
                </Button>
              ))}
            </div>
          )}
          <PostPreview
            caption={sendToTikTokInbox ? "" : (activePreview ? captionOverrides[activePreview] ?? caption : caption).trim()}
            media={activePreview === "instagram" ? media.flatMap((item, index) => {
              if (item.file.type.startsWith("video/")) return [item];
              const photo = instagramPreparation.items?.[index]?.photo;
              return photo ? [{ ...item, ...photo }] : [];
            }) : media}
            platforms={activePreview ? [activePreview] : []}
          />
        </div>
      </div>

      <Dialog open={captionEditor !== null} onOpenChange={(open) => { if (!open) setCaptionEditor(null); }}>
        {captionEditor && (
          <DialogContent style={{ maxWidth: "min(26rem, calc(100% - 2rem))" }}>
            <DialogTitle>Edit caption for {captionSettings[captionEditor.platform].name}</DialogTitle>
            <DialogDescription>
              {captionEditor.platform === "youtube"
                ? "YouTube uses the first line as the title and the rest as the description."
                : "Changes here apply only to this platform."}
            </DialogDescription>
            <Textarea
              aria-label="Custom caption"
              rows={7}
              value={captionEditor.text}
              onChange={(event) => setCaptionEditor({ ...captionEditor, text: event.target.value })}
            />
            <p className="text-xs text-muted-foreground">
              {captionEditor.text.trim().length}/{captionSettings[captionEditor.platform].limit}
            </p>
            <DialogFooter>
              {captionOverrides[captionEditor.platform] !== undefined && (
                <Button type="button" variant="outline" onClick={() => {
                  setCaptionOverrides((current) => {
                    const remaining = { ...current };
                    delete remaining[captionEditor.platform];
                    return remaining;
                  });
                  setCaptionEditor(null);
                }}>
                  Use shared caption
                </Button>
              )}
              <Button type="button" disabled={Boolean(captionError(captionEditor.platform, captionEditor.text))} onClick={() => {
                setCaptionOverrides((current) => ({ ...current, [captionEditor.platform]: captionEditor.text.trim() }));
                setCaptionEditor(null);
              }}>
                Save caption
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>

      <ScheduleDialog
        open={scheduleOpen}
        onOpenChange={setScheduleOpen}
        onSchedule={handleScheduleSubmit}
        isSubmitting={isScheduleSubmitting}
      />
    </>
  );
}


function validateInstagramMedia(format: InstagramFormat, types: string[], targets: number): string | null {
  const isVideo = (type: string) => type === "video/mp4" || type === "video/quicktime";
  const isPhoto = (type: string) => ["image/jpeg", "image/png", "image/webp"].includes(type);
  if (format === "CAROUSEL") {
    if (targets !== 1) return "Select Instagram only to publish a carousel.";
    if (types.length < 2 || types.length > 10) return "Add 2–10 photos or MP4/MOV videos in publishing order.";
    if (types.some(type => !isPhoto(type) && !isVideo(type))) return "Instagram carousels accept JPEG, PNG, or WebP photos and MP4/MOV videos.";
  } else if (types.length !== 1) {
    return format === "REELS" ? "Instagram Reels require exactly one MP4 or MOV video." : "Add one media attachment for this Instagram format.";
  } else if (format === "IMAGE" && !isPhoto(types[0])) {
    return "Add a JPEG, PNG, or WebP photo for the Instagram feed.";
  } else if (format === "REELS" && !isVideo(types[0])) {
    return "Add an MP4 or MOV video for the Reel.";
  } else if (format === "STORIES" && !isPhoto(types[0]) && !isVideo(types[0])) {
    return "Add a JPEG, PNG, or WebP photo or an MP4/MOV video for the Story.";
  }
  return null;
}
