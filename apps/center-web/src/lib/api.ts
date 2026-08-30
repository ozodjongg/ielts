export class ApiError extends Error {
  status: number;
  code: string;
  requestId?: string;
  constructor(status: number, code: string, message: string, requestId?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

const configured = (process.env.NEXT_PUBLIC_API_URL || "").trim();
if (process.env.NODE_ENV === "production" && !configured) {
  throw new Error("NEXT_PUBLIC_API_URL is required in production");
}
export const API_URL = (configured || "http://localhost:8080").replace(/\/$/, "");

function safePath(path: string) {
  if (!path.startsWith("/")) throw new Error(`API path must start with /: ${path}`);
  if (/(^|\/)(undefined|null)(\/|$)/i.test(path)) throw new Error(`Refusing invalid API path: ${path}`);
  return path;
}

function requestId() {
  try { return globalThis.crypto?.randomUUID?.() || `web-${Date.now()}`; } catch { return `web-${Date.now()}`; }
}

export async function api<T = unknown>(path: string, token = "", init: RequestInit = {}): Promise<T> {
  const finalPath = safePath(path);
  const headers = new Headers(init.headers || {});
  headers.set("Accept", "application/json");
  headers.set("X-Request-ID", headers.get("X-Request-ID") || requestId());
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (init.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json");

  const timeout = init.body instanceof FormData ? 120_000 : 30_000;
  const signal = init.signal || (typeof AbortSignal !== "undefined" && "timeout" in AbortSignal ? AbortSignal.timeout(timeout) : undefined);
  let response: Response;
  try {
    response = await fetch(API_URL + finalPath, { ...init, headers, signal, cache: "no-store", credentials: "omit" });
  } catch (error: any) {
    if (error?.name === "TimeoutError" || error?.name === "AbortError") throw new ApiError(0, "timeout", "Server javobi kutilgan vaqtda kelmadi.");
    throw new ApiError(0, "network", "Backend bilan ulanish amalga oshmadi.");
  }

  if (!response.ok) {
    let payload: any = {};
    try { payload = await response.json(); } catch {}
    throw new ApiError(
      response.status,
      payload.error || "http_error",
      payload.message || `Request failed (${response.status})`,
      payload.request_id || response.headers.get("X-Request-ID") || undefined,
    );
  }
  if (response.status === 204) return undefined as T;
  const contentType = response.headers.get("content-type") || "";
  if (!contentType.includes("application/json")) return (await response.blob()) as T;
  return response.json() as Promise<T>;
}

export async function apiBlob(path: string, token: string, headers?: HeadersInit): Promise<Blob> {
  return api<Blob>(path, token, { headers });
}

export const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) });

export function portalPath(portal: "admin" | "center" | "teacher" | "student", service: string, path = "") {
  if (!/^[a-z][a-z0-9_-]*$/.test(service)) throw new Error(`Invalid service name: ${service}`);
  const suffix = path ? (path.startsWith("/") ? path : `/${path}`) : "";
  return safePath(`/api/${portal}/${service}${suffix}`.replace(/\/$/, ""));
}
