import { apiClient } from "@/lib/api/client";
import type {
  CreatePostRequest,
  ListPostsFilter,
  ListPostsResponse,
  PostResponse,
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
