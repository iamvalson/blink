"use client";

import { useRef, useState } from "react";

export default function TestPostPage() {
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [caption, setCaption] = useState("");
  const [targetId, setTargetId] = useState("");
  const [status, setStatus] = useState("");
  const [isError, setIsError] = useState(false);
  const [loading, setLoading] = useState(false);

  function showStatus(msg: string, error = false) {
    setStatus(msg);
    setIsError(error);
  }

  async function handleUploadAndPost(e: React.FormEvent) {
    e.preventDefault();

    const currentFile = file ?? fileRef.current?.files?.[0] ?? null;

    if (!currentFile) {
      showStatus("Please select a video file.", true);
      return;
    }
    if (!targetId.trim()) {
      showStatus("Please enter a YouTube Account UUID.", true);
      return;
    }
    if (!caption.trim()) {
      showStatus("Please enter a video title / caption.", true);
      return;
    }

    setLoading(true);
    showStatus("Uploading video…");

    try {
      // 1. Upload video to local public folder
      const formData = new FormData();
      formData.append("file", currentFile);

      const uploadRes = await fetch("/api/upload", {
        method: "POST",
        credentials: "include",
        body: formData,
      });

      if (!uploadRes.ok) {
        throw new Error(
          `Upload failed (${uploadRes.status}): ${await uploadRes.text()}`,
        );
      }

      const { url } = await uploadRes.json();
      showStatus(`Uploaded to ${url}. Creating post…`);

      // 2. Create the post
      const postRes = await fetch("/api/posts", {
        method: "POST",
        credentials: "include",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": `test-post-${Date.now()}`,
        },
        body: JSON.stringify({
          caption: caption.trim(),
          media_url: url,
          media_type: currentFile.type || "video/mp4",
          targets: [targetId.trim()],
        }),
      });

      if (!postRes.ok) {
        throw new Error(
          `Post creation failed (${postRes.status}): ${await postRes.text()}`,
        );
      }

      showStatus(
        "✅ Post created! Check your worker logs to see it uploading to YouTube.",
      );
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      console.error(err);
      showStatus(`Error: ${msg}`, true);
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="dashboard-shell">
      <section className="dashboard-panel max-w-2xl">
        <h1>Test YouTube Upload</h1>
        <p style={{ marginBottom: "2rem" }}>
          Upload a video file from your computer and post it to YouTube.
        </p>

        {status && (
          <div
            style={{
              marginBottom: "1.5rem",
              padding: "1rem",
              backgroundColor: isError ? "#fbe8e3" : "#e2f1e7",
              color: isError ? "#963b2b" : "#24623e",
              borderRadius: "4px",
              fontWeight: 600,
            }}
          >
            {status}
          </div>
        )}

        <form
          onSubmit={handleUploadAndPost}
          style={{ display: "flex", flexDirection: "column", gap: "1rem" }}
        >
          <div>
            <label style={{ display: "block", marginBottom: "0.5rem" }}>
              Video File:
            </label>
            <input
              ref={fileRef}
              type="file"
              accept="video/*"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              disabled={loading}
            />
          </div>

          <div>
            <label style={{ display: "block", marginBottom: "0.5rem" }}>
              YouTube Account UUID:
            </label>
            <input
              type="text"
              placeholder="e.g. 123e4567-e89b-12d3-a456-426614174000"
              value={targetId}
              onChange={(e) => setTargetId(e.target.value)}
              disabled={loading}
              style={{ width: "100%", padding: "0.5rem" }}
            />
          </div>

          <div>
            <label style={{ display: "block", marginBottom: "0.5rem" }}>
              Video Title / Caption:
            </label>
            <input
              type="text"
              placeholder="My awesome video"
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              disabled={loading}
              style={{ width: "100%", padding: "0.5rem" }}
            />
          </div>

          <button
            type="submit"
            className="button button-primary"
            disabled={loading}
            style={{
              alignSelf: "flex-start",
              marginTop: "1rem",
              cursor: loading ? "wait" : "pointer",
            }}
          >
            {loading ? "Processing…" : "Upload & Post"}
          </button>
        </form>
      </section>
    </main>
  );
}
