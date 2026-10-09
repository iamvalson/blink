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
    };

    signup.mutate(signupData, {
      onSuccess: () => {
        router.push("/");
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
