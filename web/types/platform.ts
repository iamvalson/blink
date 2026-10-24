export type Platform = "twitter" | "youtube" | "tiktok";

export type PlatformStatus = "connected" | "disconnected" | "error";

export interface PlatformDefinition {
  id: Platform;
  name: string;
  description: string;
}

export const PLATFORMS: PlatformDefinition[] = [
  {
    id: "twitter",
    name: "X",
    description: "Publish posts and media to X.",
  },
  {
    id: "youtube",
    name: "YouTube",
    description: "Publish videos to YouTube.",
  },
  {
    id: "tiktok",
    name: "TikTok",
    description: "Publish videos to TikTok.",
  },
];
