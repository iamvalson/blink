"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useQueryClient } from "@tanstack/react-query";
import { Eye, EyeOff } from "lucide-react";
import Link from "next/link";
import { startTransition, useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError } from "@/lib/api/client";
import { resetPassword } from "../api/password-recovery";
import { readResetToken, resetPasswordSchema, type ResetPasswordFormValues } from "../schemas/reset-password";

export function ResetPasswordForm() {
  const [token, setToken] = useState<string | null | undefined>(undefined);
  const initialized = useRef(false);
  const [done, setDone] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const queryClient = useQueryClient();
  const form = useForm<ResetPasswordFormValues>({
    resolver: zodResolver(resetPasswordSchema),
    defaultValues: { password: "", confirm_password: "" },
  });

  useEffect(() => {
    // Strict Mode can run this effect twice; keep the first captured token.
    if (initialized.current) return;
    initialized.current = true;
    const value = readResetToken(window.location.hash);
    window.history.replaceState(window.history.state, "", window.location.pathname);
    startTransition(() => setToken(value));
  }, []);

  const onSubmit = async (values: ResetPasswordFormValues) => {
    if (!token) return;
    setError("");
    try {
      await resetPassword(token, values.password);
      queryClient.clear();
      form.reset();
      setToken(null);
      setDone(true);
    } catch (error) {
      if (error instanceof ApiError && error.code === "RESET_TOKEN_INVALID") {
        setToken(null);
      }
      setError(error instanceof Error ? error.message : "Could not reset your password. Try again.");
    }
  };

  if (token === undefined) {
    return <p role="status" className="text-sm text-muted-foreground">Opening reset link...</p>;
  }

  if (done) {
    return (
      <div className="w-full max-w-sm space-y-4">
        <h1 className="text-2xl font-semibold tracking-tight">Password updated</h1>
        <p role="status" className="text-sm text-muted-foreground">
          Your password has been changed and your previous sessions have been signed out. Log in with your new password.
        </p>
        <Link href="/login" className="inline-block text-sm font-medium underline">Log in</Link>
      </div>
    );
  }

  if (!token) {
    return (
      <div className="w-full max-w-sm space-y-4">
        <h1 className="text-2xl font-semibold tracking-tight">Request a new reset link</h1>
        <p role="alert" className="text-sm text-muted-foreground">
          {error || "This link is missing, invalid, or has already been opened in this tab. Open the link from your email again or request a new one."}
        </p>
        <Link href="/forgot-password" className="inline-block text-sm font-medium underline">Request a reset link</Link>
      </div>
    );
  }

  return (
    <div className="w-full max-w-sm">
      <h1 className="text-2xl font-semibold tracking-tight">Choose a new password</h1>
      <p className="mt-2 mb-8 text-sm leading-6 text-muted-foreground">
        Use 8 to 128 characters, with uppercase and lowercase letters, a number, and a symbol.
      </p>
      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
        <div className="space-y-2">
          <Label htmlFor="password">New password</Label>
          <div className="relative">
            <Input id="password" type={showPassword ? "text" : "password"} autoComplete="new-password"
              disabled={form.formState.isSubmitting} {...form.register("password")} />
            <button type="button" onClick={() => setShowPassword((current) => !current)}
              aria-label={showPassword ? "Hide password" : "Show password"}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">
              {showPassword ? <EyeOff className="size-4" /> : <Eye className="size-4" />}
            </button>
          </div>
          {form.formState.errors.password && <p role="alert" className="text-sm text-destructive">{form.formState.errors.password.message}</p>}
        </div>
        <div className="space-y-2">
          <Label htmlFor="confirm_password">Confirm password</Label>
          <Input id="confirm_password" type={showPassword ? "text" : "password"} autoComplete="new-password"
            disabled={form.formState.isSubmitting} {...form.register("confirm_password")} />
          {form.formState.errors.confirm_password && <p role="alert" className="text-sm text-destructive">{form.formState.errors.confirm_password.message}</p>}
        </div>
        {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
        <Button type="submit" className="h-10 w-full" disabled={form.formState.isSubmitting}>
          {form.formState.isSubmitting ? "Updating password..." : "Update password"}
        </Button>
      </form>
      <Link href="/forgot-password" className="mt-6 inline-block text-sm text-muted-foreground hover:underline">Request another link</Link>
    </div>
  );
}
