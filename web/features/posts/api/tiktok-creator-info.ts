import { apiClient } from "@/lib/api/client";
import type { TikTokCreatorInfo } from "../utils/tiktok-validator";
export function getTikTokCreatorInfo(accountID: string) {
 return apiClient<TikTokCreatorInfo>(`/api/v1/accounts/${encodeURIComponent(accountID)}/tiktok/creator-info`);
}
