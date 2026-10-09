"use client";

import { useEffect } from "react";
import { useSearchParams, useRouter, usePathname } from "next/navigation";
import { toast } from "sonner";
import { PLATFORMS } from "@/types/platform";

import { AccountCard } from "@/components/accounts/account-card";
import { useAccounts } from "../hooks/use-accounts";

export function AccountsList() {
  const { data, isLoading, isError, refetch } = useAccounts();
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    let shouldCleanUrl = false;
    PLATFORMS.forEach((platform) => {
      if (searchParams.get(platform.id) === "connected") {
        toast.success(`Successfully connected ${platform.name}`);
        shouldCleanUrl = true;
      }
    });

    if (shouldCleanUrl) {
      refetch();
      router.replace(pathname, { scroll: false });
    }
  }, [searchParams, pathname, router, refetch]);

  const accounts = data?.accounts ?? [];

  if (isError) {
    return (
      <div className="rounded-lg border border-destructive/20 p-6">
        <p className="font-medium">Failed to load connected accounts.</p>

        <p className="mt-1 text-sm text-muted-foreground">Please try again.</p>
      </div>
    );
  }

  return (
    <div className="grid gap-4 md:grid-cols-2">
      {PLATFORMS.map((platform) => {
        const account = accounts.find((item) => item.platform === platform.id);

        return (
          <AccountCard
            key={platform.id}
            platform={platform}
            account={account}
            isLoading={isLoading}
          />
        );
      })}
    </div>
  );
}
