import type { MediaUploadResponse } from "@/types/post";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "";

export async function uploadMedia(
  file: File,
  onProgress?: (percent: number) => void,
): Promise<MediaUploadResponse> {
  return new Promise((resolve, reject) => {
    const formData = new FormData();
    formData.append("file", file, file.name);

    const xhr = new XMLHttpRequest();
    xhr.open("POST", `${API_URL}/api/v1/media`);
    xhr.withCredentials = true;

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
        try {
          const data = JSON.parse(xhr.responseText) as MediaUploadResponse;
          resolve(data);
        } catch {
          reject(new Error("Invalid JSON response from upload server"));
        }
      } else {
        let errMsg = `Upload failed with status ${xhr.status}`;
        try {
          const data = JSON.parse(xhr.responseText);
          if (data.message || data.error) {
            errMsg = data.message || data.error;
          }
        } catch {
          if (xhr.responseText) {
            errMsg = xhr.responseText;
          }
        }
        reject(new Error(errMsg));
      }
    };

    xhr.onerror = () => {
      reject(new Error("Network error during media upload"));
    };

    xhr.ontimeout = () => {
      reject(new Error("Media upload timed out"));
    };

    xhr.send(formData);
  });
}

