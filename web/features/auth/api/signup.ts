import { apiClient } from "@/lib/api/client";
import type { SignupInput, SignupResponse } from "../types";

export async function signup(input: SignupInput): Promise<SignupResponse> {
  return apiClient<SignupResponse>("/auth/signup", {
    method: "POST",
    body: JSON.stringify(input),
  });
}
