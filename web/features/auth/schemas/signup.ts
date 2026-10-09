import { z } from "zod";

export const signupSchema = z
  .object({
    email: z.string().email("Enter a valid email address"),

    display_name: z
      .string()
      .min(2, "Display name must be at least 2 characters")
      .max(50, "Display name must be 50 characters or less"),

    password: z
      .string()
      .min(8, "Password must be at least 8 characters")
      .regex(/[A-Z]/, "Password must contain at least one uppercase letter")
      .regex(/[a-z]/, "Password must contain at least one lowercase letter")
      .regex(/[0-9]/, "Password must contain at least one number")
      .regex(
        /[^A-Za-z0-9]/,
        "Password must contain at least one special character",
      ),

    confirm_password: z.string(),
  })
  .refine((data) => data.password === data.confirm_password, {
    message: "Passwords do not match",
    path: ["confirm_password"],
  });

export type SignupFormValues = z.infer<typeof signupSchema>;
