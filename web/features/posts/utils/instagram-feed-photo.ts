import type { InstagramFormat } from "@/types/post";

export type PhotoFraming = "fit" | "crop";

const targetBytes = 7_500_000;

export function feedPhotoDimensions(sourceWidth: number, sourceHeight: number) {
  const ratio = sourceWidth / sourceHeight;
  const width = Math.min(1080, Math.max(320, sourceWidth));
  const height = Math.min(Math.floor(width * 5 / 4), Math.max(Math.ceil(width / 1.91), Math.round(width / ratio)));
  return { width, height, needsFraming: ratio < 0.8 || ratio > 1.91 };
}

async function encodeJPEG(canvas: HTMLCanvasElement): Promise<Blob> {
  for (const quality of [0.9, 0.8, 0.65, 0.5]) {
    const blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, "image/jpeg", quality));
    if (blob?.type === "image/jpeg" && blob.size <= targetBytes) return blob;
  }
  throw new Error("Blink could not compress this photo for Instagram.");
}

export function instagramPhotoDimensions(width: number, height: number, format: InstagramFormat) {
  if (format === "STORIES") return { width: 1080, height: 1920, needsFraming: width * 16 !== height * 9 };
  if (format === "CAROUSEL") return { width: 1080, height: 1080, needsFraming: width !== height };
  return feedPhotoDimensions(width, height);
}

export async function prepareInstagramFeedPhoto(source: File, format: InstagramFormat = "IMAGE") {
  const image = await createImageBitmap(source, { imageOrientation: "from-image" });
  try {
    const { width, height, needsFraming } = instagramPhotoDimensions(image.width, image.height, format);
    const canvas = document.createElement("canvas");
    canvas.width = width;
    canvas.height = height;
    const renderingContext = canvas.getContext("2d", { alpha: false });
    if (!renderingContext) throw new Error("Image preparation is unavailable in this browser.");
    const context = renderingContext;
    context.imageSmoothingQuality = "high";

    async function render(mode?: PhotoFraming) {
      const scale = mode === "crop" ? Math.max(width / image.width, height / image.height)
        : Math.min(width / image.width, height / image.height);
      const drawWidth = image.width * scale;
      const drawHeight = image.height * scale;
      context.fillStyle = "#fff";
      context.fillRect(0, 0, width, height);
      context.drawImage(image, (width - drawWidth) / 2, (height - drawHeight) / 2, drawWidth, drawHeight);
      return new File([await encodeJPEG(canvas)], "instagram-" + source.name.replace(/\.[^.]+$/, "") + ".jpg", { type: "image/jpeg" });
    }

    if (!needsFraming) return { width, height, automatic: await render() };
    const fit = await render("fit");
    const crop = await render("crop");
    return { width, height, fit, crop };
  } finally {
    image.close();
  }
}
