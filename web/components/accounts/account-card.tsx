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
    if (confirm(`Are you sure you want to disconnect ${platform.name}?`)) {
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
      <CardHeader className="flex flex-row items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-lg border">
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
            <DropdownMenuTrigger className="inline-flex items-center justify-center rounded-md text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground h-9 w-9 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring">
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

      <CardContent className="flex items-center justify-between gap-4">
        <div>
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
      </CardContent>
    </Card>
  );
}
