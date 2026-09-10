import {
  backendRequest,
  copyCookies,
  forwardCookies,
} from "@/lib/server/backend";

export async function GET(request: Request) {
  const url = new URL(request.url);
  const requestHeaders = new Headers();

  forwardCookies(request, requestHeaders);

  const response = await backendRequest(`/auth/twitter/callback${url.search}`, {
    headers: requestHeaders,
  });

  const headers = new Headers();
  copyCookies(response, headers);

  const backendLocation = response.headers.get("location");

  if (backendLocation) {
    const dashboardSearch = new URL(backendLocation, url.origin).search;
    headers.set("Location", `/dashboard${dashboardSearch}`);
  }

  return new Response(response.body, {
    status: response.status,
    headers,
  });
}
