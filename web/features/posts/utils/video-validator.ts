import type { ContentFormat } from "@/types/post";

export type VideoOrientation = "Vertical" | "Square" | "Landscape";

export interface VideoMetadata {
  duration: number; // in seconds
  width: number;
  height: number;
  aspectRatio: number; // width / height
  orientation: VideoOrientation;
  formattedDuration: string; // e.g. "01:45"
  resolution: string; // e.g. "1080 × 1920"
}

export type VideoValidationStatus = "valid" | "warning" | "error";

export interface VideoValidationResult {
  status: VideoValidationStatus;
  metadata?: VideoMetadata;
  error?: string;
  warning?: string;
  suggestion?: string;
  canSwitchToNormalVideo?: boolean;
}

/**
 * Formats a duration in seconds into a human-readable string (mm:ss or hh:mm:ss).
 * Example: 105 -> "01:45", 180 -> "03:00", 3665 -> "01:01:05"
 */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) {
    return "00:00";
  }
  const totalSeconds = Math.round(seconds);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const secs = totalSeconds % 60;

  const mm = String(minutes).padStart(2, "0");
  const ss = String(secs).padStart(2, "0");

  if (hours > 0) {
    const hh = String(hours).padStart(2, "0");
    return `${hh}:${mm}:${ss}`;
  }
  return `${mm}:${ss}`;
}

/**
 * Determines video orientation based on width and height.
 * - Vertical: height > width
 * - Square: height === width
 * - Landscape: width > height
 */
export function getOrientation(width: number, height: number): VideoOrientation {
  if (height > width) return "Vertical";
  if (height === width) return "Square";
  return "Landscape";
}

/**
 * Pure validation function that evaluates a video's metadata against
 * the selected format (Short vs Normal Video).
 */
export function validateVideoForFormat(
  metadata: VideoMetadata,
  format: ContentFormat,
): VideoValidationResult {
  if (format === "SHORT") {
    const isSquareOrVertical = metadata.height >= metadata.width;
    const isDurationEligible = metadata.duration <= 180; // YouTube Shorts allow up to 3 minutes (180s)

    // Case 1: Both orientation and duration fail
    if (!isSquareOrVertical && !isDurationEligible) {
      return {
        status: "warning",
        metadata,
        warning: `This video is in landscape orientation (${metadata.resolution}) and is ${metadata.formattedDuration} long. YouTube Shorts require vertical or square video (height ≥ width) and a maximum duration of 3 minutes (180s).`,
        suggestion:
          "Switch to Normal Video or choose a vertical/square video under 3 minutes.",
        canSwitchToNormalVideo: true,
      };
    }

    // Case 2: Orientation fails (landscape)
    if (!isSquareOrVertical) {
      return {
        status: "warning",
        metadata,
        warning: `This video has a landscape orientation (${metadata.resolution}). YouTube Shorts require vertical or square video (height ≥ width, such as 9:16).`,
        suggestion:
          "Switch to Normal Video or upload a vertical video.",
        canSwitchToNormalVideo: true,
      };
    }

    // Case 3: Duration exceeds 3 minutes (180s)
    if (!isDurationEligible) {
      return {
        status: "warning",
        metadata,
        warning: `This video is ${metadata.formattedDuration} long, which exceeds the 3-minute limit (180 seconds) for YouTube Shorts.`,
        suggestion:
          "Switch to Normal Video or trim the video to 3 minutes or less.",
        canSwitchToNormalVideo: true,
      };
    }

    // Case 4: Eligible
    return {
      status: "valid",
      metadata,
    };
  }

  // Normal Video ("VIDEO"):
  // Allows both landscape and portrait videos.
  // Does not reject a video simply because it is vertical or short.
  return {
    status: "valid",
    metadata,
  };
}

/**
 * Extracts video metadata in the browser using the HTML5 <video> element.
 * Handles timeouts, invalid formats, corrupted files, and supports AbortSignal for race conditions.
 */
export function extractVideoMetadata(
  file: File,
  signal?: AbortSignal,
): Promise<VideoMetadata> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      return reject(new DOMException("Metadata extraction aborted", "AbortError"));
    }

    if (!file.type.startsWith("video/")) {
      return reject(
        new Error(`The selected file "${file.name}" is not recognized as a video format.`),
      );
    }

    let objectUrl: string | null = null;
    try {
      objectUrl = URL.createObjectURL(file);
    } catch (err) {
      return reject(
        new Error(`Failed to create local object URL: ${err instanceof Error ? err.message : String(err)}`),
      );
    }

    const video = document.createElement("video");
    video.preload = "metadata";
    video.muted = true;
    video.playsInline = true;

    let isCleanedUp = false;
    const cleanup = () => {
      if (isCleanedUp) return;
      isCleanedUp = true;
      video.removeAttribute("src");
      video.load();
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
        objectUrl = null;
      }
      if (signal) {
        signal.removeEventListener("abort", onAbort);
      }
    };

    const onAbort = () => {
      cleanup();
      reject(new DOMException("Metadata extraction aborted", "AbortError"));
    };

    if (signal) {
      signal.addEventListener("abort", onAbort, { once: true });
    }

    // Timeout protection against hanging or stalled files (10 seconds)
    const timeoutId = setTimeout(() => {
      cleanup();
      reject(
        new Error(
          "Timed out while reading video metadata. The file may be corrupted or unsupported by your browser.",
        ),
      );
    }, 10000);

    video.onloadedmetadata = () => {
      clearTimeout(timeoutId);

      const width = video.videoWidth;
      const height = video.videoHeight;
      const duration = video.duration;

      cleanup();

      if (!width || !height || !Number.isFinite(duration) || duration <= 0) {
        return reject(
          new Error(
            "Unable to read valid dimensions or duration from the video. The file may be corrupted.",
          ),
        );
      }

      const aspectRatio = width / height;
      const orientation = getOrientation(width, height);
      const formattedDuration = formatDuration(duration);
      const resolution = `${width} × ${height}`;

      resolve({
        duration,
        width,
        height,
        aspectRatio,
        orientation,
        formattedDuration,
        resolution,
      });
    };

    video.onerror = () => {
      clearTimeout(timeoutId);
      cleanup();
      reject(
        new Error(
          `Unable to load video file "${file.name}". The format may not be supported by your browser or the file is corrupted.`,
        ),
      );
    };

    video.src = objectUrl;
  });
}
