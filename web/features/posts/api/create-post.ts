import { apiClient } from "@/lib/api/client";
import type { CreatePostRequest, PostResponse } from "@/types/post";

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
