"use client";

import { Menu } from "lucide-react";
import Link from "next/link";

import { Button } from "@/components/ui/button";
import { UserMenu } from "./user-menu";

export function MobileHeader() {
  return (
    <header className="flex h-14 items-center justify-between border-b px-4 md:hidden">
      <Link href="/" className="text-lg font-semibold tracking-tight">
        Blink
      </Link>

      <div className="flex items-center gap-1">
        <Button variant="ghost" size="icon" aria-label="Open navigation">
          <Menu className="size-5" />
        </Button>

        <UserMenu />
      </div>
    </header>
  );
}
