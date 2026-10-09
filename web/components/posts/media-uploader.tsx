"use client";

import { ImagePlus, Loader2, Trash2, Upload, AlertCircle } from "lucide-react";
import { useRef } from "react";

export type UploadStatus = "idle" | "uploading" | "complete" | "error";

export type MediaFile = {
  id: string;
  file: File;
  previewUrl: string;
  uploadStatus?: UploadStatus;
  uploadProgress?: number;
  mediaId?: string;
  error?: string;
};

type MediaUploaderProps = {
  media: MediaFile[];
  onChange: (media: MediaFile[]) => void;
  onUpload?: (localId: string, file: File) => void;
  disabled?: boolean;
};

export function MediaUploader({ media, onChange, onUpload, disabled }: MediaUploaderProps) {
  const inputRef = useRef<HTMLInputElement>(null);

  const handleFiles = (files: FileList | null) => {
    if (!files || disabled) return;

    const newFiles: MediaFile[] = Array.from(files).map((file) => ({
      id: crypto.randomUUID(),
      file,
      previewUrl: URL.createObjectURL(file),
      uploadStatus: "idle",
      uploadProgress: 0,
    }));

    onChange([...media, ...newFiles]);

    if (onUpload) {
      newFiles.forEach((item) => {
        onUpload(item.id, item.file);
      });
    }
  };

  const removeFile = (id: string) => {
    const fileToRemove = media.find((item) => item.id === id);

    if (fileToRemove) {
      URL.revokeObjectURL(fileToRemove.previewUrl);
    }

    onChange(media.filter((item) => item.id !== id));
  };

  return (
    <div className="space-y-3">
      <div>
        <h3 className="text-sm font-medium">Media</h3>
        <p className="mt-1 text-sm text-muted-foreground">
          Add images or videos to your post.
        </p>
      </div>

      {media.length > 0 && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
          {media.map((item) => {
            const isVideo = item.file.type.startsWith("video/");
            const isUploading = item.uploadStatus === "uploading";
            const isError = item.uploadStatus === "error";

            return (
              <div
                key={item.id}
                className="group relative aspect-square overflow-hidden rounded-lg border bg-muted"
              >
                {isVideo ? (
                  <video
                    src={item.previewUrl}
                    className="h-full w-full object-cover"
                    muted
                  />
                ) : (
                  <img
                    src={item.previewUrl}
                    alt={item.file.name}
                    className="h-full w-full object-cover"
                  />
                )}

                {/* Uploading progress overlay */}
                {isUploading && (
                  <div className="absolute inset-0 flex flex-col items-center justify-center bg-black/60 p-2 text-white">
                    <Loader2 className="h-6 w-6 animate-spin" />
                    <span className="mt-2 text-xs font-medium">
                      {item.uploadProgress !== undefined ? `${item.uploadProgress}%` : "Uploading..."}
                    </span>
                  </div>
                )}

                {/* Error overlay */}
                {isError && (
                  <div className="absolute inset-0 flex flex-col items-center justify-center bg-destructive/80 p-2 text-white">
                    <AlertCircle className="h-6 w-6" />
                    <span className="mt-1 text-center text-xs line-clamp-2">
                      {item.error || "Upload failed"}
                    </span>
                  </div>
                )}

                <button
                  type="button"
                  onClick={() => removeFile(item.id)}
                  disabled={isUploading}
                  className="absolute right-2 top-2 flex h-8 w-8 items-center justify-center rounded-full bg-black/70 text-white opacity-0 transition-opacity group-hover:opacity-100 disabled:opacity-0"
                  aria-label={`Remove ${item.file.name}`}
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              </div>
            );
          })}
        </div>
      )}

      <button
        type="button"
        disabled={disabled}
        onClick={() => inputRef.current?.click()}
        className="flex w-full flex-col items-center justify-center rounded-lg border border-dashed px-6 py-8 text-center transition-colors hover:border-foreground/30 hover:bg-muted/40 disabled:opacity-50 disabled:pointer-events-none"
      >
        <div className="mb-3 flex h-10 w-10 items-center justify-center rounded-full bg-muted">
          {media.length > 0 ? (
            <ImagePlus className="h-5 w-5" />
          ) : (
            <Upload className="h-5 w-5" />
          )}
        </div>

        <p className="text-sm font-medium">
          {media.length > 0 ? "Add more media" : "Upload media"}
        </p>

        <p className="mt-1 text-xs text-muted-foreground">
          PNG, JPG, GIF, MP4 and other supported formats
        </p>

        <input
          ref={inputRef}
          type="file"
          accept="image/*,video/*"
          multiple
          disabled={disabled}
          className="hidden"
          onChange={(event) => {
            handleFiles(event.target.files);

            if (inputRef.current) {
              inputRef.current.value = "";
            }
          }}
        />
      </button>
    </div>
  );
}
