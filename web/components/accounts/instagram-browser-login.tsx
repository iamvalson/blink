"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { apiClient } from "@/lib/api/client";

export function InstagramBrowserLogin({ disabled = false }: { disabled?: boolean }) {
  const [url, setUrl] = useState("");
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState("");

  async function createLink() {
    setLoading(true);
    setUrl("");
    setMessage("");
    try {
      const { authorization_url } = await apiClient<{ authorization_url: string }>("/auth/instagram/browser-link", { method: "POST" });
      setUrl(authorization_url);
    } catch {
      setMessage("Could not create a login link. Refresh Blink and try again.");
    } finally {
      setLoading(false);
    }
  }

  async function copyLink() {
    try {
      await navigator.clipboard.writeText(url);
      setMessage("Link copied. Paste it into the Safari address bar and tap Go.");
    } catch {
      setMessage("Select and copy the link below, then paste it into the Safari address bar.");
    }
  }

  return (
    <details className="w-full text-sm text-muted-foreground">
      <summary className="cursor-pointer">Instagram app opens with an error?</summary>
      <div className="mt-2 space-y-2">
        <p>Create a login link, then paste it directly into the Safari address bar. Use the link within 10 minutes. If the app still opens, connect from desktop and return to Accounts on this device.</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" variant="outline" disabled={disabled || loading} onClick={createLink}>
            {loading ? "Creating link…" : url ? "Create a new link" : "Create Safari login link"}
          </Button>
          {url && <Button type="button" size="sm" variant="outline" onClick={copyLink}>Copy link</Button>}
        </div>
        {url && <input aria-label="Instagram login link" readOnly value={url} onFocus={event => event.currentTarget.select()} className="w-full rounded border bg-background p-2 text-xs" />}
        {message && <p role="status">{message}</p>}
      </div>
    </details>
  );
}
