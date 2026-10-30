"use client";

import { siX, siYoutube, siTiktok } from "simple-icons";

import type { Platform } from "@/types/platform";

interface PlatformIconProps {
  platform: Platform;
  size?: number;
  className?: string;
}

export function PlatformIcon({
  platform,
  size = 20,
  className,
}: PlatformIconProps) {
  switch (platform) {
    case "twitter":
      return (
        <svg
          role="img"
          viewBox="0 0 24 24"
          width={size}
          height={size}
          className={className}
          aria-label="X"
        >
          <path d={siX.path} />
        </svg>
      );

    case "youtube":
      return (
        <svg
          role="img"
          viewBox="0 0 24 24"
          width={size}
          height={size}
          className={className}
          aria-label="YouTube"
        >
          <path d={siYoutube.path} />
        </svg>
      );

    case "tiktok":
      return (
        <svg
          role="img"
          viewBox="0 0 24 24"
          width={size}
          height={size}
          className={className}
          aria-label="TikTok"
        >
          <path d={siTiktok.path} />
        </svg>
      );

    default:
      return null;
  }
}
