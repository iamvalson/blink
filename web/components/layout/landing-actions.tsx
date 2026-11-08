"use client";

import Link from "next/link";
import { ArrowUpRight } from "lucide-react";
import { useCurrentUser } from "@/features/auth/hooks/use-current-user";

const primary = "inline-flex min-h-11 items-center justify-center gap-2 rounded-full bg-white px-5 py-2 text-sm font-bold text-black transition-colors hover:bg-neutral-200";

export function LandingActions({ navigation = false }: { navigation?: boolean }) {
  const { data: user, isPending } = useCurrentUser();

  return (
    <div className="flex min-h-11 items-center gap-3 sm:gap-5">
      {isPending ? (
        <span aria-hidden="true" className="h-11 w-32 rounded-full bg-white/10" />
      ) : user ? (
        <Link href="/dashboard" className={primary}>
          Go to dashboard <ArrowUpRight className="size-4" aria-hidden="true" />
        </Link>
      ) : (
        <>
          {navigation && <Link href="/login" className="flex min-h-11 items-center text-sm font-medium text-white/80 hover:text-white">Log in</Link>}
          <Link href="/signup" className={primary}>
            {navigation ? "Sign up" : "Create an account"} <ArrowUpRight className="size-4" aria-hidden="true" />
          </Link>
        </>
      )}
    </div>
  );
}
