"use client";

import { MoreHorizontal } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { SocialAccount } from "@/types/account";

import { PlatformIcon } from "./platform-icon";

interface AccountRowProps {
  account: SocialAccount;
}

export function AccountRow({ account }: AccountRowProps) {
  return (
    <div className="flex items-center justify-between gap-4 border-b py-4 last:border-0">
      <div className="flex items-center gap-3">
        <div className="flex size-9 items-center justify-center rounded-lg border">
          <PlatformIcon platform={account.platform} size={18} />
        </div>

        <div>
          <p className="font-medium capitalize">
            {account.platform} Account
          </p>

          <p className="text-sm text-muted-foreground">{account.platform_user_id}</p>
        </div>
      </div>

      <Button variant="ghost" size="icon">
        <MoreHorizontal className="size-4" />
        <span className="sr-only">Account options</span>
      </Button>
    </div>
  );
}
