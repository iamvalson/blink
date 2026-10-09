"use client";

import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { PlatformIcon } from "./platform-icon";

import type { Platform } from "@/types/platform";

interface ConnectPlatformProps {
  platform: Platform;
  disabled?: boolean;
  isLoading?: boolean;
}

const API_URL = process.env.NEXT_PUBLIC_API_URL;

const OAUTH_PATHS: Record<Platform, string> = {
  twitter: "/auth/twitter",
  youtube: "/auth/youtube",
};

export function ConnectPlatform({
  platform,
  disabled = false,
  isLoading = false,
}: ConnectPlatformProps) {
  function handleConnect() {
    const path = OAUTH_PATHS[platform];

    window.location.href = `${API_URL}${path}`;
  }

  return (
    <Button
      type="button"
      onClick={handleConnect}
      disabled={disabled || isLoading}
    >
      {isLoading ? (
        <Loader2 className="size-4 animate-spin" />
      ) : (
        <PlatformIcon platform={platform} size={16} />
      )}

      {isLoading ? "Connecting..." : "Connect"}
    </Button>
  );
}
