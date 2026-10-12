import type { Platform } from "@/types/platform";

export interface ConnectAccountResponse {
  authorizationUrl: string;
  platform: Platform;
}

export interface DisconnectAccountResponse {
  success: boolean;
}
