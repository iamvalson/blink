"use client";

import { Loader2, RefreshCcw } from "lucide-react";

import { Button, buttonVariants } from "@/components/ui/button";
import { PlatformIcon } from "./platform-icon";

import type { Platform } from "@/types/platform";

interface ConnectPlatformProps {
  platform: Platform;
  disabled?: boolean;
  isLoading?: boolean;
  reconnect?: boolean;
  reconnectRequired?: boolean;
}

export function platformOAuthLink(platform: Platform) {
  const apiUrl = process.env.NEXT_PUBLIC_API_URL || "";
  return {
    href: `${apiUrl}/auth/${platform}`,
    ...(platform === "instagram" ? { target: "_blank", rel: "noopener noreferrer" } : {}),
  };
}

export function ConnectPlatform({
  platform,
  disabled = false,
  isLoading = false,
  reconnect = false,
  reconnectRequired = false,
}: ConnectPlatformProps) {
  const variant = reconnect && !reconnectRequired ? "outline" : "default";
  const size = reconnect ? "sm" : "default";
  const className = reconnectRequired ? "bg-amber-600 hover:bg-amber-700 text-white" : undefined;
  const label = reconnect ? "Reconnect" : "Connect";
  const link = platformOAuthLink(platform);
  const icon = isLoading ? <Loader2 className="size-4 animate-spin" /> :
    reconnect ? <RefreshCcw className="size-3.5" /> : <PlatformIcon platform={platform} size={16} decorative />;

  if (disabled || isLoading) {
    return <Button type="button" variant={variant} size={size} className={className} disabled>{icon}{label}</Button>;
  }

  return (
    <a {...link} className={buttonVariants({ variant, size, className })}>
      {icon}
      {label}
      {link.target === "_blank" && <span className="sr-only"> Instagram in a new tab</span>}
    </a>
  );
}
