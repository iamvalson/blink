const backendUrl = process.env.BACKEND_API_URL ?? "http://localhost:8080";

export function backendRequest(path: string, init: RequestInit = {}) {
  const initHeaders = new Headers(init.headers);
  // Bypass the ngrok browser interstitial page on server-to-server requests.
  initHeaders.set("ngrok-skip-browser-warning", "true");

  return fetch(`${backendUrl}${path.startsWith("/") ? path : `/${path}`}`, {
    ...init,
    headers: initHeaders,
    cache: "no-store",
    redirect: "manual",
  });
}

export function forwardCookies(request: Request, headers: Headers) {
  const cookie = request.headers.get("cookie");
  if (cookie) headers.set("Cookie", cookie);
}

export function copyCookies(response: Response, headers: Headers) {
  const cookies =
    "getSetCookie" in response.headers &&
    typeof response.headers.getSetCookie === "function"
      ? response.headers.getSetCookie()
      : [];

  for (const cookie of cookies) {
    const normalized =
      process.env.NODE_ENV === "production"
        ? cookie
        : cookie.replace(/;\s*Secure/gi, "");

    headers.append("Set-Cookie", normalized);
  }
}
