"use client";

import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { UserMenu } from "./user-menu";

export function MobileHeader() {
  const pathname = usePathname();
  const isPostScreen = pathname?.startsWith("/posts/");

  return (
    <header className="mobile-app-header sticky top-0 z-30 border-b bg-background lg:hidden">
      <div className="flex h-14 items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          {isPostScreen && (
            <Link
              href="/posts"
              aria-label="Back to posts"
              className="-ml-2 flex size-11 items-center justify-center rounded-lg hover:bg-muted focus-visible:outline-2"
            >
              <ArrowLeft className="size-5" aria-hidden="true" />
            </Link>
          )}
          <Link href="/dashboard" className="flex min-h-11 items-center text-xl font-semibold tracking-tight">
            Blink
          </Link>
        </div>
        <UserMenu />
      </div>
    </header>
  );
}
