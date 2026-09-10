import { backendRequest, copyCookies } from "@/lib/server/backend";

export async function POST(request: Request) {
  const body = await request.text();
  const contentType = request.headers.get("content-type") ?? "";
  const isJsonRequest = contentType.includes("application/json");
  const payload = isJsonRequest
    ? body
    : JSON.stringify(Object.fromEntries(new URLSearchParams(body)));

  const response = await backendRequest("/auth/signup", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: payload,
  });

  const headers = new Headers({
    "Content-Type": "application/json",
  });

  copyCookies(response, headers);

  if (!isJsonRequest && response.ok) {
    headers.set("Location", "/dashboard");
    return new Response(null, { status: 303, headers });
  }

  return new Response(response.body, {
    status: response.status,
    headers,
  });
}
