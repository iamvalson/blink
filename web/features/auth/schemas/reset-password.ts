import { z } from "zod";

export const resetPasswordSchema = z.object({
  password: z.string()
    .refine((value) => Array.from(value).length >= 8 && Array.from(value).length <= 128,
      "Use between 8 and 128 characters")
    .regex(/\p{Lu}/u, "Include an uppercase letter")
    .regex(/\p{Ll}/u, "Include a lowercase letter")
    .regex(/\p{N}/u, "Include a number")
    .regex(/[\p{P}\p{S}]/u, "Include a symbol"),
  confirm_password: z.string(),
}).refine((value) => value.password === value.confirm_password, {
  message: "Passwords do not match", path: ["confirm_password"],
});

export type ResetPasswordFormValues = z.infer<typeof resetPasswordSchema>;

// Fragments are not transmitted in HTTP requests or Referer headers.
export function readResetToken(fragment: string): string | null {
  if (!fragment.startsWith("#")) return null;
  const token = new URLSearchParams(fragment.replace(/^#/, "")).get("token");
  return token && /^[A-Za-z0-9_-]{43}$/.test(token) ? token : null;
}
