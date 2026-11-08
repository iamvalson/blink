"use client";

import { CalendarDays, FileText, House, Plus, Users } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

const destinations = [
  { label: "Overview", href: "/dashboard", icon: House },
  { label: "Posts", href: "/posts", icon: FileText },
  { label: "Create", href: "/posts/new", icon: Plus },
  { label: "Calendar", href: "/calendar", icon: CalendarDays },
  { label: "Accounts", href: "/accounts", icon: Users },
];

export function MobileNav() {
  const pathname = usePathname();
  const activeHref = pathname?.startsWith("/posts/") && pathname !== "/posts/new"
    ? "/posts"
    : pathname;

  return (
    <nav
      aria-label="Mobile navigation"
      className="mobile-tab-bar fixed inset-x-0 bottom-0 z-40 border-t bg-background lg:hidden"
    >
      <ul className="mx-auto grid max-w-lg grid-cols-5">
        {destinations.map(({ label, href, icon: Icon }) => {
          const active = href === activeHref;
          const current = active ? (href === pathname ? "page" : "location") : undefined;
          return (
            <li key={href} className="min-w-0">
              <Link
                href={href}
                aria-current={current}
                className="flex h-[var(--mobile-tab-height)] flex-col items-center justify-center gap-0.5 rounded-lg px-0.5 text-[11px] font-medium leading-tight text-muted-foreground transition-colors hover:text-foreground focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring aria-[current]:font-semibold aria-[current]:text-foreground"
              >
                <span className={
                  label === "Create"
                    ? "flex h-8 w-11 items-center justify-center rounded-xl bg-primary text-primary-foreground"
                    : active
                      ? "flex h-8 w-11 items-center justify-center rounded-xl bg-muted"
                      : "flex h-8 w-11 items-center justify-center"
                }>
                  <Icon className="size-5" aria-hidden="true" />
                </span>
                {label}
              </Link>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}
