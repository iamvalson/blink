import { apiClient } from "@/lib/api/client";

export async function logout(): Promise<void> {
  await apiClient<void>("/auth/logout", {
    method: "POST",
  });
}
