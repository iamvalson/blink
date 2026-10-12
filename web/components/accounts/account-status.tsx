import { CheckCircle2, CircleAlert, Loader2 } from "lucide-react";

import type { SocialAccount } from "@/types/account";

interface AccountStatusProps {
  account?: SocialAccount;
  isLoading?: boolean;
}

export function AccountStatus({
  account,
  isLoading = false,
}: AccountStatusProps) {
  if (isLoading) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" />
        Checking...
      </div>
    );
  }

  if (!account) {
    return (
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        <span className="size-2 rounded-full bg-muted-foreground/40" />
        Not connected
      </div>
    );
  }

  return (
    <div className="flex items-center gap-2 text-sm text-emerald-600">
      <CheckCircle2 className="size-4" />
      Connected
    </div>
  );
}
