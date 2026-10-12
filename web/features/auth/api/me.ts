import { apiClient } from "@/lib/api/client";
import type { User } from "../types";

interface MeResponse {
  user_id: string;
  email: string;
  display_name: string;
}

export async function getCurrentUser(): Promise<User> {
  const response = await apiClient<MeResponse>("/auth/me");

  return {
    id: response.user_id,
    email: response.email,
    display_name: response.display_name,
  };
}
