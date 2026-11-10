"use client";

import { siX, siYoutube, siTiktok, siInstagram } from "simple-icons";

const icons = new Map([
  ["twitter", siX],
  ["youtube", siYoutube],
  ["tiktok", siTiktok],
  ["instagram", siInstagram],
]);

interface PlatformIconProps {
  platform: string;
  size?: number;
  className?: string;
  decorative?: boolean;
}

export function PlatformIcon({ platform, size = 20, className, decorative = false }: PlatformIconProps) {
  const icon = icons.get(platform);
  if (!icon) return null;
  return (
    <svg
      role={decorative ? undefined : "img"}
      aria-hidden={decorative || undefined}
      aria-label={decorative ? undefined : icon.title}
      viewBox="0 0 24 24"
      width={size}
      height={size}
      fill="currentColor"
      className={className}
    >
      <path d={icon.path} />
    </svg>
  );
}
