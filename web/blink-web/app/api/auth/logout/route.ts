import {
  backendRequest,
  copyCookies,
  forwardCookies,
} from "@/lib/server/backend";

export async function POST(request: Request) {
  const requestHeaders = new Headers();
  forwardCookies(request, requestHeaders);

  const response = await backendRequest("/auth/logout", {
    method: "POST",
    headers: requestHeaders,
  });

  const headers = new Headers();
  copyCookies(response, headers);

  return new Response(null, {
    status: response.status,
    headers,
  });
}
