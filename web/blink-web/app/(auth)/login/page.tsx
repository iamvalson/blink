"use client";

import { useRouter } from "next/navigation";
import { SyntheticEvent, useState } from "react";

export default function LoginPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setLoading(true);

    const form = new FormData(event.currentTarget);

    const response = await fetch("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        email: form.get("email"),
        password: form.get("password"),
      }),
    });

    setLoading(false);

    if (!response.ok) {
      setError((await response.text()) || "Unable to sign in");
      return;
    }

    router.replace("/dashboard");
  }

  return (
    <main className="auth-shell">
      <section className="auth-panel">
        <p className="eyebrow">Blink</p>
        <h1>Welcome back</h1>
        <p>Sign in to manage your connected accounts.</p>

        <form
          action="/api/auth/login"
          method="post"
          onSubmit={submit}
          className="auth-form"
        >
          <label htmlFor="email">Email</label>
          <input id="email" name="email" type="email" required />

          <label htmlFor="password">Password</label>
          <input id="password" name="password" type="password" required />

          {error && <p role="alert">{error}</p>}

          <button type="submit" disabled={loading}>
            {loading ? "Signing in..." : "Sign in"}
          </button>
        </form>

        <a href="/signup">Create an account</a>
      </section>
    </main>
  );
}
