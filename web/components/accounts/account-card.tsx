"use client";

import { MoreHorizontal, RefreshCcw, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

import type { SocialAccount } from "@/types/account";
import type { PlatformDefinition } from "@/types/platform";

import { useDisconnectAccount } from "@/features/accounts/hooks/use-disconnect-account";
import { AccountStatus } from "./account-status";
import { ConnectPlatform } from "./connect-platform";
import { PlatformIcon } from "./platform-icon";
import { InstagramBrowserLogin } from "./instagram-browser-login";

interface AccountCardProps {
  platform: PlatformDefinition;
  account?: SocialAccount;
  isLoading?: boolean;
}

export function AccountCard({
  platform,
  account,
  isLoading = false,
}: AccountCardProps) {
  const disconnectMutation = useDisconnectAccount();

  function handleReconnect() {
    const apiUrl = process.env.NEXT_PUBLIC_API_URL || "";
    window.location.href = `${apiUrl}/auth/${platform.id}`;
  }

  function handleDisconnect() {
    const message = `Disconnect ${platform.name}? This revokes Blink's access and removes publishing records for this connection from Blink.`;
    if (confirm(platform.id === "instagram" ? "Disconnect Instagram and remove its publishing records from Blink?" : message)) {
      disconnectMutation.mutate(platform.id, {
        onSuccess: () => {
          toast.success(`Successfully disconnected ${platform.name}`);
        },
        onError: () => {
          toast.error(`Failed to disconnect ${platform.name}`);
        },
      });
    }
  }

  return (
    <Card className={account?.reconnect_required ? "border-amber-500/50" : undefined}>
      <CardHeader className="flex flex-row items-start justify-between gap-2 px-4 sm:px-6">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg border">
            <PlatformIcon platform={platform.id} size={20} />
          </div>

          <div>
            <h3 className="font-medium">{platform.name}</h3>

            <p className="text-sm text-muted-foreground">
              {platform.description}
            </p>
          </div>
        </div>

        {account && (
          <DropdownMenu>
            <DropdownMenuTrigger className="inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground size-11 shrink-0 lg:size-9 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring">
              <MoreHorizontal className="size-4" />
              <span className="sr-only">Account options</span>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem onClick={handleReconnect}>
                <RefreshCcw className="mr-2 size-4" />
                Reconnect
              </DropdownMenuItem>
              <DropdownMenuItem
                variant="destructive"
                onClick={handleDisconnect}
                disabled={disconnectMutation.isPending}
              >
                <Trash2 className="mr-2 size-4" />
                Disconnect
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </CardHeader>

      <CardContent className="flex flex-wrap items-center justify-between gap-3 px-4 sm:px-6 [&_button]:min-h-11 lg:[&_button]:min-h-0">
        <div>
          {account?.username && <p className="text-sm font-medium">@{account.username}</p>}
          <AccountStatus account={account} isLoading={isLoading} />
        </div>

        {!account ? (
          <ConnectPlatform platform={platform.id} isLoading={isLoading} />
        ) : (
          <Button
            type="button"
            variant={account.reconnect_required ? "default" : "outline"}
            size="sm"
            onClick={handleReconnect}
            disabled={isLoading}
            className={account.reconnect_required ? "bg-amber-600 hover:bg-amber-700 text-white" : ""}
          >
            <RefreshCcw className="mr-1.5 size-3.5" />
            Reconnect
          </Button>
        )}
        {platform.id === "instagram" && <InstagramBrowserLogin disabled={isLoading} />}
      </CardContent>
    </Card>
  );
}
