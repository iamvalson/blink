"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { Check, Eye, EyeOff, X } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";
import { useForm, useWatch } from "react-hook-form";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SIGNUP_LEGAL_VERSION } from "@/lib/signup-legal-version";

import { useAuth } from "../hooks/use-auth";
import { signupSchema, type SignupFormValues } from "../schemas/signup";

export function SignupForm() {
  const router = useRouter();
  const { signup } = useAuth();

  const [showPassword, setShowPassword] = useState(false);
  const [showConfirmPassword, setShowConfirmPassword] = useState(false);

  const form = useForm<SignupFormValues>({
    resolver: zodResolver(signupSchema),
    defaultValues: {
      email: "",
      display_name: "",
      password: "",
      confirm_password: "",
      accept_terms: false,
    },
  });

  const password = useWatch({ control: form.control, name: "password" });
  const confirmPassword = useWatch({
    control: form.control,
    name: "confirm_password",
  });

  const passwordRequirements = [
    {
      label: "Use 8 or more characters",
      valid: password.length >= 8,
    },
    {
      label: "One uppercase letter",
      valid: /[A-Z]/.test(password),
    },
    {
      label: "One lowercase letter",
      valid: /[a-z]/.test(password),
    },
    {
      label: "One number",
      valid: /[0-9]/.test(password),
    },
    {
      label: "One symbol",
      valid: /[^A-Za-z0-9]/.test(password),
    },
  ];

  const passwordsMatch =
    confirmPassword.length > 0 && password === confirmPassword;

  const onSubmit = (values: SignupFormValues) => {
    const signupData = {
      email: values.email,
      display_name: values.display_name,
      password: values.password,
      accept_terms: values.accept_terms,
      legal_version: SIGNUP_LEGAL_VERSION,
    };

    signup.mutate(signupData, {
      onSuccess: () => {
        router.replace("/dashboard");
      },
    });
  };

  return (
    <div className="w-full max-w-sm">
      <div className="mb-8">
        <h1 className="text-2xl font-semibold tracking-tight">
          Create your account
        </h1>

        <p className="mt-2 text-sm text-muted-foreground">
          Start managing your social content with Blink.
        </p>
      </div>

      <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-5">
        <div className="space-y-2">
          <Label htmlFor="display_name">Name</Label>

          <Input
            id="display_name"
            placeholder="Your name"
            autoComplete="name"
            {...form.register("display_name")}
          />

          {form.formState.errors.display_name && (
            <p className="text-sm text-destructive">
              {form.formState.errors.display_name.message}
            </p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="email">Email</Label>

          <Input
            id="email"
            type="email"
            placeholder="you@example.com"
            autoComplete="email"
            {...form.register("email")}
          />

          {form.formState.errors.email && (
            <p className="text-sm text-destructive">
              {form.formState.errors.email.message}
            </p>
          )}
        </div>

        {/* Password */}
        <div className="space-y-2">
          <Label htmlFor="password">Password</Label>

          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              placeholder="••••••••"
              autoComplete="new-password"
              {...form.register("password")}
            />

            <button
              type="button"
              onClick={() => setShowPassword((current) => !current)}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
              aria-label={showPassword ? "Hide password" : "Show password"}
            >
              {showPassword ? (
                <EyeOff className="size-4" />
              ) : (
                <Eye className="size-4" />
              )}
            </button>
          </div>

          <div className="space-y-1.5 pt-1">
            {passwordRequirements.map((requirement) => (
              <div
                key={requirement.label}
                className="flex items-center gap-2 text-sm"
              >
                {requirement.valid ? (
                  <Check className="size-4 shrink-0 text-green-600" />
                ) : (
                  <X className="size-4 shrink-0 text-muted-foreground" />
                )}

                <span
                  className={
                    requirement.valid
                      ? "text-green-600"
                      : "text-muted-foreground"
                  }
                >
                  {requirement.label}
                </span>
              </div>
            ))}
          </div>

          {form.formState.errors.password && (
            <p className="text-sm text-destructive">
              {form.formState.errors.password.message}
            </p>
          )}
        </div>

        {/* Confirm password */}
        <div className="space-y-2">
          <Label htmlFor="confirm_password">Confirm password</Label>

          <div className="relative">
            <Input
              id="confirm_password"
              type={showConfirmPassword ? "text" : "password"}
              placeholder="••••••••"
              autoComplete="new-password"
              {...form.register("confirm_password")}
            />

            <button
              type="button"
              onClick={() => setShowConfirmPassword((current) => !current)}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
              aria-label={
                showConfirmPassword
                  ? "Hide confirm password"
                  : "Show confirm password"
              }
            >
              {showConfirmPassword ? (
                <EyeOff className="size-4" />
              ) : (
                <Eye className="size-4" />
              )}
            </button>
          </div>

          {confirmPassword.length > 0 && (
            <div className="flex items-center gap-2 text-sm">
              {passwordsMatch ? (
                <>
                  <Check className="size-4 text-green-600" />
                  <span className="text-green-600">Passwords match</span>
                </>
              ) : (
                <>
                  <X className="size-4 text-muted-foreground" />
                  <span className="text-muted-foreground">
                    Passwords do not match
                  </span>
                </>
              )}
            </div>
          )}

          {form.formState.errors.confirm_password && (
            <p className="text-sm text-destructive">
              {form.formState.errors.confirm_password.message}
            </p>
          )}
        </div>

        <div className="rounded-lg border bg-muted/40 p-3">
          <div className="flex items-start gap-3">
            <input
              id="accept_terms"
              type="checkbox"
              className="mt-0.5 size-5 shrink-0 accent-primary"
              aria-required="true"
              aria-invalid={Boolean(form.formState.errors.accept_terms)}
              aria-describedby={form.formState.errors.accept_terms ? "signup-legal-error" : undefined}
              {...form.register("accept_terms")}
            />
            <label htmlFor="accept_terms" className="min-h-11 text-sm leading-6">
              I agree to the{" "}
              <Link href="/terms" target="_blank" rel="noopener noreferrer" className="font-medium underline underline-offset-2">
                Terms of Service
              </Link>{" "}
              and acknowledge the{" "}
              <Link href="/privacy" target="_blank" rel="noopener noreferrer" className="font-medium underline underline-offset-2">
                Privacy Policy
              </Link>.
            </label>
          </div>
          {form.formState.errors.accept_terms && (
            <p id="signup-legal-error" role="alert" className="mt-2 text-sm text-destructive">
              {form.formState.errors.accept_terms.message}
            </p>
          )}
        </div>

        {signup.isError && (
          <p className="text-sm text-destructive">{signup.error.message}</p>
        )}

        <Button
          type="submit"
          className="w-full h-10"
          disabled={signup.isPending}
        >
          {signup.isPending ? "Creating account..." : "Create account"}
        </Button>
      </form>

      <p className="mt-7 text-center text-sm text-muted-foreground border-t border-[#E7E7E7]">
        <br />
        Already have an account?{" "}
        <Link
          href="/login"
          className="font-medium text-foreground underline-offset-4 hover:underline"
        >
          Log in
        </Link>
      </p>
    </div>
  );
}
