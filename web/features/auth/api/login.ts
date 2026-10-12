import { apiClient } from "@/lib/api/client";
import type { LoginInput, LoginResponse } from "../types";

export async function login(input: LoginInput): Promise<LoginResponse> {
  return apiClient<LoginResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
