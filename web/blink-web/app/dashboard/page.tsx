"use client";

import { useSearchParams } from "next/navigation";
import { Suspense } from "react";

function DashboardContent() {
  const params = useSearchParams();

  async function logout() {
    const response = await fetch("/api/auth/logout", {
      method: "POST",
      credentials: "include",
      cache: "no-store",
    });

    if (response.ok) {
      window.location.replace("/login");
    }
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
        {params.get("youtube") === "connected" && (
          <p>YouTube connected successfully.</p>
        )}

        <div className="flex gap-4">
          <a className="button" href="/api/auth/twitter">
            Connect Twitter
          </a>

          <a
            className="button"
            href="/api/auth/youtube"
            style={{ backgroundColor: "#ff0000", color: "white" }}
          >
            Connect YouTube
          </a>
        </div>

        <button
          className="button"
          onClick={logout}
          type="button"
          style={{ marginTop: "2rem" }}
        >
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
