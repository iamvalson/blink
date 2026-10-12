"use client";

import { Check } from "lucide-react";
import { siX, siYoutube } from "simple-icons";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type Platform = "twitter" | "youtube";

type PlatformSelectorProps = {
  selectedPlatforms: Platform[];
  onToggle: (platform: Platform) => void;
  connectedPlatforms?: Platform[];
};

const platforms: {
  id: Platform;
  name: string;
  description: string;
  icon: typeof siX;
}[] = [
  {
    id: "twitter",
    name: "X",
    description: "Post to X",
    icon: siX,
  },
  {
    id: "youtube",
    name: "YouTube",
    description: "Upload to YouTube",
    icon: siYoutube,
  },
];

export function PlatformSelector({
  selectedPlatforms,
  onToggle,
  connectedPlatforms = ["twitter", "youtube"],
}: PlatformSelectorProps) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {platforms.map((platform) => {
        const selected = selectedPlatforms.includes(platform.id);
        const connected = connectedPlatforms.includes(platform.id);
        const Icon = platform.icon;

        return (
          <Button
            key={platform.id}
            type="button"
            variant="outline"
            onClick={() => onToggle(platform.id)}
            disabled={!connected}
            className={cn(
              "relative h-auto justify-start gap-3 px-4 py-3 text-left",
              selected && "border-primary bg-primary/5",
              !connected && "cursor-not-allowed opacity-50",
            )}
          >
            <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border bg-background">
              <svg
                role="img"
                viewBox="0 0 24 24"
                className="h-4 w-4 fill-current"
                aria-hidden="true"
              >
                <path d={Icon.path} />
              </svg>
            </div>

            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium">{platform.name}</p>
              <p className="text-xs text-muted-foreground">
                {platform.description}
              </p>
            </div>

            <div
              className={cn(
                "flex h-5 w-5 items-center justify-center rounded-full border",
                selected && "border-primary bg-primary text-primary-foreground",
              )}
            >
              {selected && <Check className="h-3 w-3" />}
            </div>
          </Button>
        );
      })}
    </div>
  );
}
