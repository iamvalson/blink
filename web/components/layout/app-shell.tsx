import { MobileNav } from "./mobile-nav";
import { MobileHeader } from "./mobile-header";
import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-dvh bg-background">
      <Sidebar />

      <div className="flex min-w-0 flex-1 flex-col lg:ml-60">
        <MobileHeader />

        <div className="hidden lg:block">
          <Topbar />
        </div>

        <main className="mobile-app-main min-w-0 flex-1 lg:pt-14">{children}</main>
        <MobileNav />
      </div>
    </div>
  );
}
