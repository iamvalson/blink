import { MobileHeader } from "./mobile-header";
import { Sidebar } from "./sidebar";
import { Topbar } from "./topbar";

export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-screen bg-background">
      <Sidebar />

      <div className="flex min-w-0 flex-1 flex-col md:ml-60">
        <MobileHeader />

        <div className="hidden md:block">
          <Topbar />
        </div>

        <main className="min-w-0 flex-1 md:pt-14">{children}</main>
      </div>
    </div>
  );
}
