import { apiClient } from "@/lib/api/client";
import type {
  CreatePostRequest,
  ListPostsFilter,
  ListPostsResponse,
  PostResponse,
  ReschedulePostRequest,
} from "@/types/post";

export async function createPost(
  input: CreatePostRequest,
): Promise<PostResponse> {
  const idempotencyKey = crypto.randomUUID();

  return apiClient<PostResponse>("/api/v1/posts", {
    method: "POST",
    headers: {
      "Idempotency-Key": idempotencyKey,
    },
    body: JSON.stringify(input),
  });
}

export async function getPost(postId: string): Promise<PostResponse> {
  return apiClient<PostResponse>(`/api/v1/posts/${postId}`);
}

export async function listPosts(
  filter: ListPostsFilter = {},
): Promise<ListPostsResponse> {
  const params = new URLSearchParams();
  if (filter.status) params.set("status", filter.status);
  if (filter.platform) params.set("platform", filter.platform);
  if (filter.search) params.set("search", filter.search);
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));

  const query = params.toString();
  return apiClient<ListPostsResponse>(
    `/api/v1/posts${query ? `?${query}` : ""}`,
  );
}

export async function deletePost(postId: string): Promise<void> {
  return apiClient<void>(`/api/v1/posts/${postId}`, {
    method: "DELETE",
  });
}

export async function retryPost(postId: string): Promise<PostResponse> {
  return apiClient<PostResponse>(`/api/v1/posts/${postId}/retry`, {
    method: "POST",
  });
}

export async function reschedulePost(
  postId: string,
  body: ReschedulePostRequest,
): Promise<PostResponse> {
  return apiClient<PostResponse>(`/api/v1/posts/${postId}/schedule`, {
    method: "PATCH",
    body: JSON.stringify(body),
  });
}

export async function cancelSchedule(postId: string): Promise<PostResponse> {
  return apiClient<PostResponse>(`/api/v1/posts/${postId}/schedule`, {
    method: "DELETE",
  });
}

export async function listCalendarPosts(
  from: Date,
  to: Date,
  status?: string,
  platform?: string,
): Promise<PostResponse[]> {
  const params = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
  });
  if (status) params.set("status", status);
  if (platform) params.set("platform", platform);
  return apiClient<PostResponse[]>(`/api/v1/calendar?${params.toString()}`);
}

export async function listUpcomingPosts(
  filter: { limit?: number; offset?: number } = {},
): Promise<ListPostsResponse> {
  const params = new URLSearchParams();
  if (filter.limit !== undefined) params.set("limit", String(filter.limit));
  if (filter.offset !== undefined) params.set("offset", String(filter.offset));
  const query = params.toString();
  return apiClient<ListPostsResponse>(
    `/api/v1/posts/upcoming${query ? `?${query}` : ""}`,
  );
}
