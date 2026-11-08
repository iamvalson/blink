"use client";

import { useEffect, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useCurrentUser } from "../hooks/use-current-user";

export function GuestGuard({ children }: { children: ReactNode }) {
  const router = useRouter();
  const { data: user, isPending } = useCurrentUser();

  useEffect(() => {
    if (user) router.replace("/dashboard");
  }, [router, user]);

  if (isPending || user) return <p role="status" className="text-sm text-muted-foreground">Loading…</p>;

  return children;
}
