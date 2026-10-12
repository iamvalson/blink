import { apiClient } from "@/lib/api/client";
import type { ConnectedAccountsResponse } from "@/types/account";

export async function getAccounts(): Promise<ConnectedAccountsResponse> {
  return apiClient<ConnectedAccountsResponse>("/api/v1/accounts", {
    method: "GET",
  });
}
