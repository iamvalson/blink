import { apiClient } from "@/lib/api/client";
import type { InitiateUploadResponse, MediaUploadResponse } from "@/types/post";

export async function uploadMedia(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<MediaUploadResponse> {
  // 1. Initiate presigned upload authorization
  const initRes = await apiClient<InitiateUploadResponse>("/api/v1/media/uploads", {
    method: "POST",
    body: JSON.stringify({
      filename: file.name,
      contentType: file.type || "application/octet-stream",
      size: file.size,
    }),
  });

  // 2. Direct upload to Cloudflare R2 presigned URL
  await new Promise<void>((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    const method = initRes.method || "PUT";
    xhr.open(method, initRes.upload_url);

    // Set signed headers (e.g., Content-Type, Content-Length if specified)
    if (initRes.headers) {
      Object.entries(initRes.headers).forEach(([k, v]) => {
        xhr.setRequestHeader(k, v);
      });
    }

    if (onProgress) {
      xhr.upload.onprogress = (event) => {
        if (event.lengthComputable) {
          const percent = Math.round((event.loaded / event.total) * 100);
          onProgress(percent);
        }
      };
    }

    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve();
      } else {
        let errMsg = `Direct upload failed with status ${xhr.status}`;
        if (xhr.responseText) {
          errMsg += `: ${xhr.responseText}`;
        }
        reject(new Error(errMsg));
      }
    };

    xhr.onerror = () => {
      reject(new Error("Network error during direct storage upload. Check CORS settings."));
    };

    xhr.ontimeout = () => {
      reject(new Error("Storage upload timed out"));
    };

    xhr.send(file);
  });

  // 3. Confirm upload and mark media as uploaded & verified
  const completeRes = await apiClient<MediaUploadResponse>(
    `/api/v1/media/${initRes.id}/complete`,
    {
      method: "POST",
    },
  );

  return completeRes;
}

export async function deleteMedia(mediaId: string): Promise<void> {
  await apiClient<void>(`/api/v1/media/${mediaId}`, {
    method: "DELETE",
  });
}


