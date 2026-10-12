"use client";

import { Search } from "lucide-react";
import { usePathname } from "next/navigation";

import { UserMenu } from "./user-menu";

const pageTitles = [
  { path: "/posts/new", title: "Posts / New" },
  { path: "/posts", title: "Posts" },
  { path: "/calendar", title: "Calendar" },
  { path: "/accounts", title: "Accounts" },
  { path: "/analytics", title: "Analytics" },
  { path: "/activity", title: "Activity" },
  { path: "/settings", title: "Settings" },
  { path: "/support", title: "Help & support" },
];

export function Topbar() {
  const pathname = usePathname();
  const pageTitle =
    pageTitles.find(({ path }) => pathname.startsWith(path))?.title ??
    "Overview";

  return (
    <header className="fixed left-60 right-0 top-0 z-30 flex h-14 shrink-0 items-center justify-between border-b bg-background px-4 md:px-6">
      <div className="text-[13px] font-medium">{pageTitle}</div>
      <div className="flex flex-row items-center gap-2">
        <div className="flex h-9 w-64 items-center gap-2 rounded-md border border-[#E7E7E7] bg-white px-3 ">
          <Search className="size-4" />
          <input
            type="search"
            placeholder="Search..."
            aria-label="Search"
            className="min-w-0 flex-1 bg-transparent text-sm text-foreground outline-none placeholder:text-muted-foreground"
          />

          <kbd className="ml-auto hidden rounded border bg-muted px-1.5 py-0.5 text-[10px] font-medium sm:inline-block">
            ⌘ K
          </kbd>
        </div>

        <UserMenu />
      </div>
    </header>
  );
}
