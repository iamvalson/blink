import {
  backendRequest,
  copyCookies,
  forwardCookies,
} from "@/lib/server/backend";

export async function GET(request: Request) {
  const requestHeaders = new Headers();
  forwardCookies(request, requestHeaders);

  const response = await backendRequest("/auth/twitter", {
    headers: requestHeaders,
  });

  const location = response.headers.get("location");

  if (!location) {
    return new Response(await response.text(), {
      status: response.status,
    });
  }

  const headers = new Headers({
    Location: location,
  });

  copyCookies(response, headers);

  return new Response(null, {
    status: response.status,
    headers,
  });
}
