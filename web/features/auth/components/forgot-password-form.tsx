"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowLeft } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { useForm } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { requestPasswordReset } from "../api/password-recovery";
import { forgotPasswordSchema, type ForgotPasswordFormValues } from "../schemas/forgot-password";

export function ForgotPasswordForm() {
  const [sent, setSent] = useState(false);
  const [error, setError] = useState("");
  const form = useForm<ForgotPasswordFormValues>({
    resolver: zodResolver(forgotPasswordSchema),
    defaultValues: { email: "" },
  });

  const onSubmit = async (values: ForgotPasswordFormValues) => {
    setError("");
    try {
      await requestPasswordReset(values.email);
      setSent(true);
    } catch (error) {
      setError(error instanceof Error ? error.message : "Could not request a reset link. Try again.");
    }
  };

  return (
    <div className="w-full max-w-sm">
      <div className="mb-8">
        <h1 className="text-2xl font-semibold tracking-tight">
          {sent ? "Check your email" : "Forgot your password?"}
        </h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {sent
            ? "If an account exists for that email, a reset link will be sent. Check your inbox and spam folder. The link expires in 30 minutes."
            : "Enter your account email and we’ll send you a link to reset your password."}
        </p>
      </div>

      {sent ? (
        <div role="status" className="space-y-4">
          <p className="text-sm text-muted-foreground">If you requested another link, use the newest email. Please wait a minute before requesting again.</p>
          <Button type="button" variant="outline" className="w-full" onClick={() => setSent(false)}>
            Request another link
          </Button>
        </div>
      ) : (
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input id="email" type="email" placeholder="you@example.com" autoComplete="email"
              maxLength={255} disabled={form.formState.isSubmitting} {...form.register("email")} />
            {form.formState.errors.email && (
              <p role="alert" className="text-sm text-destructive">{form.formState.errors.email.message}</p>
            )}
          </div>
          {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
          <Button type="submit" className="h-10 w-full" disabled={form.formState.isSubmitting}>
            {form.formState.isSubmitting ? "Requesting link..." : "Send reset link"}
          </Button>
        </form>
      )}
      <div className="mt-6 text-center">
        <Link href="/login" className="inline-flex items-center gap-1 text-sm font-medium text-muted-foreground hover:underline">
          <ArrowLeft className="size-4" /> Back to login
        </Link>
      </div>
    </div>
  );
}
