"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { Suspense } from "react";

function DashboardContent() {
  const router = useRouter();
  const params = useSearchParams();

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
  }

  return (
    <main className="dashboard-shell">
      <section className="dashboard-panel">
        <p className="eyebrow">Blink workspace</p>
        <h1>Accounts</h1>

        <p>Your authentication session is active.</p>

        {params.get("twitter") === "connected" && (
          <p>Twitter connected successfully.</p>
        )}

        <a className="button" href="/api/auth/twitter">
          Connect Twitter
        </a>

        <button className="button" onClick={logout}>
          Sign out
        </button>
      </section>
    </main>
  );
}

export default function DashboardPage() {
  return (
    <Suspense fallback={<main className="dashboard-shell" />}>
      <DashboardContent />
    </Suspense>
  );
}
