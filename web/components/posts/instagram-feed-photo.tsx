"use client";

import { useEffect, useEffectEvent, useId, useState } from "react";
import { prepareInstagramFeedPhoto, type PhotoFraming } from "@/features/posts/utils/instagram-feed-photo";
import type { InstagramFormat } from "@/types/post";
import type { MediaFile } from "./media-uploader";

type Photo = { file: File; previewUrl: string };
type Preparation = {
  id: string;
  width?: number;
  height?: number;
  automatic?: Photo;
  fit?: Photo;
  crop?: Photo;
  error?: string;
};

export function useInstagramMedia(media: MediaFile[], format: InstagramFormat, enabled: boolean) {
  // Upload progress updates the media records but does not change the input files.
  const key = enabled ? JSON.stringify([format, ...media.map(item => item.id)]) : "";
  const [result, setResult] = useState<{ key: string; items: Preparation[] }>();
  const [choices, setChoices] = useState<Record<string, PhotoFraming>>({});
  const getMedia = useEffectEvent(() => ({ media, format }));
  useEffect(() => {
    if (!key) return;
    const controller = new AbortController();
    const input = getMedia();
    const urls: string[] = [];
    let stored = false;
    function preview(file?: File) {
      if (!file) return;
      const previewUrl = URL.createObjectURL(file);
      urls.push(previewUrl);
      return { file, previewUrl };
    }
    async function prepare() {
      const items: Preparation[] = [];
      for (const { id, file } of input.media) {
        if (controller.signal.aborted) return;
        try {
          if (file.type.startsWith("video/")) {
            items.push({ id });
          } else {
            const { width, height, automatic, fit, crop } = await prepareInstagramFeedPhoto(file, input.format);
            if (controller.signal.aborted) return;
            items.push({ id, width, height, automatic: preview(automatic), fit: preview(fit), crop: preview(crop) });
          }
        } catch {
          items.push({ id, error: "Blink could not prepare this photo. Try another JPEG, PNG, or WebP." });
        }
      }
      if (!controller.signal.aborted) {
        stored = true;
        setResult({ key, items });
      }
    }
    void prepare();
    return () => {
      controller.abort();
      if (!stored) for (const url of urls) URL.revokeObjectURL(url);
    };
  }, [key]);

  useEffect(() => () => {
    for (const item of result?.items ?? []) {
      for (const photo of [item.automatic, item.fit, item.crop]) {
        if (photo) URL.revokeObjectURL(photo.previewUrl);
      }
    }
  }, [result]);

  const current = key && result?.key === key ? result.items : undefined;
  const items = current?.map((item, index) => {
    const choiceKey = JSON.stringify([format, item.id]);
    const mode = choices[choiceKey];
    const photo = item.automatic ?? (mode ? item[mode] : undefined);
    const { file, metadata } = media[index];
    let error = item.error;
    if (file.type.startsWith("video/")) {
      const maxDuration = format === "REELS" ? 900 : 60;
      if (file.size > 1_000_000_000) error = "Video must be no larger than 1 GB.";
      else if (metadata && (metadata.duration < 3 || metadata.duration > maxDuration || metadata.width > 1920)) {
        error = `Video must be 3–${maxDuration} seconds long and no wider than 1920 px.`;
      }
    }
    return { ...item, error, index, mode, photo, choose: (next: PhotoFraming) =>
      setChoices(previous => ({ ...previous, [choiceKey]: next })) };
  });
  const firstProblem = items?.find(item => item.error || (item.fit && !item.photo));
  const error = !enabled ? null : !items ? "Checking your Instagram attachments…" :
    firstProblem ? `Instagram item ${firstProblem.index + 1}: ${firstProblem.error ?? "choose how to frame this photo."}` : null;
  return { items, error };
}

export function InstagramPreparedPhoto({ item, disabled }: {
  item: NonNullable<ReturnType<typeof useInstagramMedia>["items"]>[number];
  disabled: boolean;
}) {
  const group = useId();
  const { mode, photo, choose } = item;
  if (!item.width || item.error) return null;
  return (
    <fieldset disabled={disabled} className="space-y-2 rounded-lg border p-3">
      <legend className="px-1 text-sm font-medium">Instagram photo {item.index + 1}</legend>
      <p className="text-sm text-muted-foreground">JPEG · {item.width} × {item.height} px{photo && ` · ${(photo.file.size / 1_000_000).toFixed(2)} MB`}</p>
      {item.automatic ? (
        <PhotoPreview url={item.automatic.previewUrl} alt="Prepared Instagram photo" />
      ) : (
        <div className="grid grid-cols-2 gap-3">
          {(["fit", "crop"] as const).map(option => {
            const image = item[option];
            return image && (
              <label key={option} className="space-y-2 rounded border p-2 text-sm">
                <span className="flex items-center gap-2">
                  <input type="radio" name={group} checked={mode === option} onChange={() => choose(option)} />
                  {option === "fit" ? "Fit with padding" : "Crop to fill"}
                </span>
                <PhotoPreview url={image.previewUrl} alt={option === "fit" ? "Full image with padding" : "Centered crop"} />
              </label>
            );
          })}
        </div>
      )}
    </fieldset>
  );
}

function PhotoPreview({ url, alt }: { url: string; alt: string }) {
  // This local preview is the JPEG uploaded when the post is submitted.
  // eslint-disable-next-line @next/next/no-img-element
  return <img src={url} alt={alt} className="max-h-48 w-full rounded bg-muted object-contain" />;
}
