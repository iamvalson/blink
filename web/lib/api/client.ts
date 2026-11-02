const API_URL = process.env.NEXT_PUBLIC_API_URL;

if (!API_URL) {
  throw new Error("NEXT_PUBLIC_API_URL is not configured");
}

export class ApiError extends Error {
  code?: string;
  statusCode: number;
  platform?: string;
  reconnectUrl?: string;

  constructor(
    message: string,
    statusCode: number,
    options?: { code?: string; platform?: string; reconnectUrl?: string },
  ) {
    super(message);
    this.name = "ApiError";
    this.statusCode = statusCode;
    this.code = options?.code;
    this.platform = options?.platform;
    this.reconnectUrl = options?.reconnectUrl;
  }
}

export async function apiClient<T>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`${API_URL}${endpoint}`, {
    ...options,
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      "ngrok-skip-browser-warning": "true",
      ...options.headers,
    },
  });

  if (!response.ok) {
    const contentType = response.headers.get("content-type");

    let message = "Something went wrong";
    let code: string | undefined;
    let platform: string | undefined;
    let reconnectUrl: string | undefined;

    if (contentType?.includes("application/json")) {
      const error = await response.json();
      message = error.error ?? error.message ?? message;
      code = error.code;
      platform = error.platform;
      reconnectUrl = error.reconnect_url;

      if (code === "ACCOUNT_RECONNECT_REQUIRED" && typeof window !== "undefined") {
        const dest = reconnectUrl
          ? (reconnectUrl.startsWith("http") ? reconnectUrl : `${API_URL}${reconnectUrl}`)
          : `/accounts?reconnect=${platform || ""}`;
        window.location.href = dest;
      }
    } else {
      message = (await response.text()) || message;
    }

    throw new ApiError(message, response.status, {
      code,
      platform,
      reconnectUrl,
    });
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return response.json();
}
