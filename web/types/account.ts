import type { Platform } from "./platform";

export interface SocialAccount {
  id: string;
  user_id: string;
  platform: Platform;
  platform_user_id: string;
  expires_at?: string;
  reconnect_required?: boolean;
  created_at: string;
  updated_at: string;
}

export interface ConnectedAccountsResponse {
  accounts: SocialAccount[];
}
