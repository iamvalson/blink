import { backendRequest, forwardCookies } from "@/lib/server/backend";

export async function POST(request: Request) {
  const requestHeaders = new Headers();
  forwardCookies(request, requestHeaders);

  // Forward the idempotency key if present
  const idempotencyKey =
    request.headers.get("Idempotency-Key") || `test-${Date.now()}`;
  requestHeaders.set("Idempotency-Key", idempotencyKey);
  requestHeaders.set("Content-Type", "application/json");

  const body = await request.text();

  const response = await backendRequest("/api/v1/posts", {
    method: "POST",
    headers: requestHeaders,
    body,
  });

  return new Response(await response.text(), {
    status: response.status,
    headers: {
      "Content-Type":
        response.headers.get("Content-Type") || "application/json",
    },
  });
}
