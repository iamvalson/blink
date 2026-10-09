"use client";

import { useCurrentUser } from "@/features/auth/hooks/use-current-user";
import {
  Activity,
  BarChart3,
  CalendarDays,
  CircleQuestionMark,
  Ellipsis,
  FileText,
  LayoutDashboard,
  Settings,
  UserPlus,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

const navigation = [
  {
    label: "Overview",
    href: "/",
    icon: LayoutDashboard,
  },
  {
    label: "Posts",
    href: "/posts",
    icon: FileText,
  },
  {
    label: "Calendar",
    href: "/calendar",
    icon: CalendarDays,
  },
  {
    label: "Accounts",
    href: "/accounts",
    icon: UserPlus,
  },
  {
    label: "Analytics",
    href: "/analytics",
    icon: BarChart3,
  },
];

const systemNav = [
  {
    label: "Activity",
    href: "/activity",
    icon: Activity,
  },
  {
    label: "Settings",
    href: "/settings",
    icon: Settings,
  },
];

function getInitials(displayName: string) {
  return displayName
    .split(" ")
    .filter(Boolean)
    .map((namePart) => namePart[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();
}

export function Sidebar() {
  const pathname = usePathname();
  const { data: user } = useCurrentUser();
  const displayName = user?.display_name ?? "Loading...";
  const initials = user ? getInitials(user.display_name) : "";

  return (
    <aside className="fixed inset-y-0 left-0 z-40 hidden h-screen w-60 shrink-0 border-r bg-background px-2.75 py-3.5 md:flex md:flex-col">
      <div className="flex h-14 items-center px-5">
        <Link href="/" className="text-lg font-semibold tracking-tight">
          Blink
        </Link>
      </div>

      <div className="flex min-h-0 flex-1 flex-col justify-between">
        <div className="flex flex-col gap-4.5">
          <nav className="flex-1 space-y-1 py-3">
            <h2 className="uppercase text-[10px] px-2.5 text-grey font-semibold tracking-wider">
              Workspace
            </h2>
            {navigation.map((item) => {
              const Icon = item.icon;

              const isActive =
                item.href === "/"
                  ? pathname === "/"
                  : pathname.startsWith(item.href);

              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={[
                    "flex items-center gap-3 rounded-md px-3 py-2 text-[13px] transition-colors",
                    isActive
                      ? "bg-muted font-medium text-foreground"
                      : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                  ].join(" ")}
                >
                  <Icon className="size-4" />
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </nav>
          <nav className="flex-1 space-y-1 py-3 border-t border-[#E7E7E7]">
            <h2 className="uppercase text-[10px] px-2.5 text-grey font-semibold tracking-wider mt-1">
              System
            </h2>
            {systemNav.map((item) => {
              const Icon = item.icon;

              const isActive =
                item.href === "/"
                  ? pathname === "/"
                  : pathname.startsWith(item.href);

              return (
                <Link
                  key={item.href}
                  href={item.href}
                  className={[
                    "flex items-center gap-3 rounded-md px-3 py-2 text-[13px] transition-colors",
                    isActive
                      ? "bg-muted font-medium text-foreground"
                      : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
                  ].join(" ")}
                >
                  <Icon className="size-4" />
                  <span>{item.label}</span>
                </Link>
              );
            })}
          </nav>
        </div>

        <div className="border-t py-3 flex flex-col space-y-2.5">
          <Link
            href="/support"
            className={[
              "flex items-center gap-3 rounded-md px-2 py-2 text-sm transition-colors",
              pathname.startsWith("/settings")
                ? "bg-muted font-medium text-foreground"
                : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
            ].join(" ")}
          >
            <CircleQuestionMark className="size-4" />
            <span className="text-[13px]">Help & support</span>
          </Link>

          <div className="flex flex-row justify-between items-center">
            <div className="flex flex-row gap-1.5">
              <div className="w-8 h-8 bg-[#DFE2DC] rounded-full flex justify-center items-center">
                <span className="text-[10px] font-semibold">{initials}</span>
              </div>
              <div className="flex flex-col">
                <h4 className="text-xs font-medium">{displayName}</h4>
                <p className="text-[11px] text-grey">{user?.email ?? ""}</p>
              </div>
            </div>

            <Ellipsis className="size-4" />
          </div>
        </div>
      </div>
    </aside>
  );
}
