/**
 * X (Twitter) media attachment constraints:
 *   - Up to 4 photos (image/jpeg, image/png, image/webp, image/gif(static))
 *   - OR 1 animated GIF  (image/gif with animation)
 *   - OR 1 video         (video/*)
 *   - Mixing types is not permitted.
 *
 * Source: https://developer.x.com/en/docs/x-api/posts/manage-posts/api-reference/post-posts
 */

export type XMediaKind = "photo" | "gif" | "video";

export interface XMediaConstraintError {
  kind: "count" | "mix" | "type";
  message: string;
}

/** Map a MIME type to the X media kind.  Returns null for unsupported types. */
export function xMediaKind(mimeType: string): XMediaKind | null {
  if (mimeType.startsWith("video/")) return "video";
  if (mimeType === "image/gif") return "gif";
  if (mimeType.startsWith("image/")) return "photo";
  return null;
}

/**
 * Returns null when the selection is valid for X, or an error object
 * describing the first violation found.
 */
export function validateXMediaConstraints(
  mimeTypes: string[],
): XMediaConstraintError | null {
  if (mimeTypes.length === 0) return null;

  const kinds = mimeTypes.map(xMediaKind);

  // Unsupported type
  const badIdx = kinds.findIndex((k) => k === null);
  if (badIdx !== -1) {
    return {
      kind: "type",
      message: `"${mimeTypes[badIdx]}" is not supported by X. Attach photos (JPEG, PNG, WebP), animated GIFs, or videos.`,
    };
  }

  const photos = kinds.filter((k) => k === "photo").length;
  const gifs = kinds.filter((k) => k === "gif").length;
  const videos = kinds.filter((k) => k === "video").length;
  const total = mimeTypes.length;

  // Mixing
  const presentKinds = [photos > 0, gifs > 0, videos > 0].filter(Boolean).length;
  if (presentKinds > 1) {
    return {
      kind: "mix",
      message:
        "X does not allow mixing photos, animated GIFs, and videos in a single post.",
    };
  }

  // Count limits
  if (videos > 1) {
    return { kind: "count", message: "X allows at most 1 video per post." };
  }
  if (gifs > 1) {
    return {
      kind: "count",
      message: "X allows at most 1 animated GIF per post.",
    };
  }
  if (photos > 4) {
    return { kind: "count", message: "X allows at most 4 photos per post." };
  }
  if (total > 4) {
    return {
      kind: "count",
      message: "X allows at most 4 media items per post.",
    };
  }

  return null;
}

/** Human-readable description of the X media limit for the given kind. */
export function xMediaLimitLabel(kind: XMediaKind): string {
  switch (kind) {
    case "photo":
      return "4 photos";
    case "gif":
      return "1 animated GIF";
    case "video":
      return "1 video";
  }
}

