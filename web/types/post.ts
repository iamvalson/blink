import type { TikTokOptions } from "@/features/posts/utils/tiktok-validator";
export type ContentFormat = "VIDEO" | "SHORT";

export type InstagramFormat = "IMAGE" | "REELS" | "CAROUSEL" | "STORIES";

export interface CreatePostRequest {
  instagram_media_ids?: string[];
  instagram_formats?: Record<string, InstagramFormat>;
  tiktok_options?: Record<string, TikTokOptions>;
  caption: string;
  caption_overrides?: Record<string, string>;
  targets: string[];
  media_url?: string | null;
  media_type?: string | null;
  media_id?: string | null;
  media_ids?: string[] | null;
  mediaIds?: string[] | null;
  content_format?: ContentFormat;
  title?: string | null;
  scheduled_at?: string | null;
}

export interface InitiateUploadResponse {
  id: string;
  storage_key: string;
  upload_url: string;
  method: string;
  headers: Record<string, string>;
  expires_at: string;
}

export interface MediaUploadResponse {
  id: string;
  content_type: string;
  file_size: number;
  filename: string;
  upload_status?: string;
}

export interface PostTarget {
  platform_avatar_url?: string;
  platform_username?: string;
  instagram_format?: InstagramFormat;
  caption_override?: string;
  send_to_tiktok_inbox?: boolean;
  id: string;
  social_account_id: string;
  platform: string;
  platform_user_id: string;
  status: string;
  platform_post_id?: string;
  platform_url?: string;
  error_code?: string;
  error_message?: string;
  published_at?: string;
}

export interface PostAttachment {
  position: number;
  media_id?: string;
  content_type?: string;
}

export interface PostResponse {
  instagram_media?: PostAttachment[];
  id: string;
  user_id: string;
  caption?: string;
  title?: string | null;
  media_url?: string | null;
  media_type?: string | null;
  media_id?: string | null;
  status: string;
  content_format: ContentFormat;
  targets?: PostTarget[];
  scheduled_at?: string | null;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

export interface ReschedulePostRequest {
  scheduled_at: string;
}

export interface ListPostsResponse {
  posts: PostResponse[];
  total: number;
  limit: number;
  offset: number;
}

export interface ListPostsFilter {
  status?: string;
  platform?: string;
  search?: string;
  limit?: number;
  offset?: number;
}
