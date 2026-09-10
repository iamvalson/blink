"use client";

import { useRouter } from "next/navigation";
import { SyntheticEvent, useState } from "react";

export default function SignupPage() {
  const router = useRouter();
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  async function submit(event: SyntheticEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setLoading(true);

    const form = new FormData(event.currentTarget);

    const response = await fetch("/api/auth/signup", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        display_name: form.get("display_name"),
        email: form.get("email"),
        password: form.get("password"),
      }),
    });

    setLoading(false);

    if (!response.ok) {
      setError((await response.text()) || "Unable to create account");
      return;
    }

    router.replace("/dashboard");
  }

  return (
    <main className="auth-shell">
      <section className="auth-panel">
        <p className="eyebrow">Blink</p>
        <h1>Create your account</h1>
        <p>Create a workspace for your social accounts.</p>

        <form
          action="/api/auth/signup"
          method="post"
          onSubmit={submit}
          className="auth-form"
        >
          <label htmlFor="display_name">Display name</label>
          <input id="display_name" name="display_name" required />

          <label htmlFor="email">Email</label>
          <input id="email" name="email" type="email" required />

          <label htmlFor="password">Password</label>
          <input
            id="password"
            name="password"
            type="password"
            minLength={8}
            required
          />

          {error && <p role="alert">{error}</p>}

          <button type="submit" disabled={loading}>
            {loading ? "Creating account..." : "Create account"}
          </button>
        </form>

        <a href="/login">Already have an account? Sign in</a>
      </section>
    </main>
  );
}
