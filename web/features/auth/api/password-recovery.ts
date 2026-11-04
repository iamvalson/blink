import { apiClient } from "@/lib/api/client";

export async function requestPasswordReset(email: string): Promise<{ message: string }> {
  return apiClient("/auth/forgot-password", {
    method: "POST",
    body: JSON.stringify({ email }),
  });
}

export async function resetPassword(token: string, password: string): Promise<void> {
  return apiClient("/auth/reset-password", {
    method: "POST",
    body: JSON.stringify({ token, password }),
  });
}
