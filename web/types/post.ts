export interface CreatePostRequest {
  caption: string;
  targets: string[];
  media_url?: string | null;
  media_type?: string | null;
  media_id?: string | null;
}

export interface MediaUploadResponse {
  id: string;
  content_type: string;
  file_size: number;
  filename: string;
}

export interface PostTarget {
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

export interface PostResponse {
  id: string;
  user_id: string;
  caption?: string;
  media_url?: string | null;
  media_type?: string | null;
  media_id?: string | null;
  status: string;
  targets?: PostTarget[];
  created_at: string;
  updated_at: string;
}
