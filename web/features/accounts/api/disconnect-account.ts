import { apiClient } from "@/lib/api/client";
import type { Platform } from "@/types/platform";

export async function disconnectAccount(platform: Platform): Promise<void> {
  return apiClient<void>(`/api/v1/accounts/${platform}`, {
    method: "DELETE",
  });
}
