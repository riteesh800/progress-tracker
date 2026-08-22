export const API_BASE = import.meta.env.VITE_API_BASE ?? "";

const ACCESS = "pt_access";
const REFRESH = "pt_refresh";

export function getAccess() {
  return localStorage.getItem(ACCESS);
}
export function getRefresh() {
  return localStorage.getItem(REFRESH);
}
export function setTokens(access: string, refresh: string) {
  localStorage.setItem(ACCESS, access);
  localStorage.setItem(REFRESH, refresh);
}
export function clearTokens() {
  localStorage.removeItem(ACCESS);
  localStorage.removeItem(REFRESH);
}

type ErrorBody = { error?: { message?: string; code?: string } };

async function parse(res: Response) {
  const text = await res.text();
  try {
    return text ? JSON.parse(text) : {};
  } catch {
    return { error: { message: text } };
  }
}

export class ApiError extends Error {
  status: number;
  code?: string;
  constructor(status: number, message: string, code?: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

let wakingCallback: ((waking: boolean) => void) | null = null;
export function onWaking(cb: (waking: boolean) => void) {
  wakingCallback = cb;
}

let refreshInFlight: Promise<boolean> | null = null;
let redirectingToLogin = false;

function sendToLogin() {
  if (redirectingToLogin || window.location.pathname === "/login") return;
  redirectingToLogin = true;
  window.location.assign("/login");
}

type ApiOptions = RequestInit & {
  auth?: boolean;
  timeoutMs?: number;
  _retried?: boolean;
};

/** Strip custom fields so fetch only gets valid RequestInit. */
function toRequestInit(options: ApiOptions, headers: Headers, signal: AbortSignal): RequestInit {
  const { auth: _a, timeoutMs: _t, _retried: _r, headers: _h, signal: _s, ...rest } = options;
  return { ...rest, headers, signal };
}

async function ensureAccessToken(): Promise<string | null> {
  if (refreshInFlight) {
    await refreshInFlight;
  }
  const existing = getAccess();
  if (existing) return existing;
  const refreshed = await tryRefresh();
  if (!refreshed) return null;
  return getAccess();
}

export async function api<T>(path: string, options: ApiOptions = {}): Promise<T> {
  const headers = new Headers();
  // Copy caller headers without inheriting a stale Authorization value.
  if (options.headers) {
    new Headers(options.headers).forEach((value, key) => {
      if (key.toLowerCase() !== "authorization") headers.set(key, value);
    });
  }
  if (!(options.body instanceof FormData) && !headers.has("Content-Type") && options.body) {
    headers.set("Content-Type", "application/json");
  }

  const needsAuth = options.auth !== false;
  if (needsAuth) {
    const access = await ensureAccessToken();
    if (!access) {
      sendToLogin();
      throw new ApiError(401, "Please log in again.", "unauthenticated");
    }
    headers.set("Authorization", `Bearer ${access}`);
  }

  const timeoutMs = options.timeoutMs ?? 45000;
  const controller = new AbortController();
  const wakeTimer = window.setTimeout(() => wakingCallback?.(true), 8000);
  const abortTimer = window.setTimeout(() => controller.abort(), timeoutMs);
  let res: Response;
  try {
    res = await fetch(`${API_BASE}${path}`, toRequestInit(options, headers, controller.signal));
  } catch {
    wakingCallback?.(false);
    throw new ApiError(0, "Could not reach the server. It may be waking up — try again in a moment.");
  } finally {
    window.clearTimeout(wakeTimer);
    window.clearTimeout(abortTimer);
    wakingCallback?.(false);
  }

  if (res.status === 401 && needsAuth && !options._retried) {
    const ok = await tryRefresh();
    if (ok && getAccess()) {
      return api<T>(path, { ...options, headers: undefined, _retried: true });
    }
    clearTokens();
    sendToLogin();
    throw new ApiError(401, "Session expired. Please log in again.", "unauthenticated");
  }

  const data = await parse(res);
  if (!res.ok) {
    const err = data as ErrorBody;
    throw new ApiError(res.status, err.error?.message || "Request failed", err.error?.code);
  }
  return data as T;
}

export async function tryRefresh(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = doRefresh().finally(() => {
    refreshInFlight = null;
  });
  return refreshInFlight;
}

async function doRefresh(): Promise<boolean> {
  const refresh = getRefresh();
  if (!refresh) return false;
  let res: Response;
  try {
    res = await fetch(`${API_BASE}/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refresh_token: refresh }),
    });
  } catch {
    return Boolean(getAccess());
  }
  if (!res.ok) {
    // Another tab/caller may already have rotated and stored a newer pair.
    if (getRefresh() && getRefresh() !== refresh && getAccess()) return true;
    // Do not wipe tokens if a newer refresh landed while this request failed.
    if (getRefresh() && getRefresh() !== refresh) return Boolean(getAccess());
    clearTokens();
    return false;
  }
  const data = (await res.json()) as { access_token?: string; refresh_token?: string };
  if (!data.access_token || !data.refresh_token) {
    return Boolean(getAccess());
  }
  setTokens(data.access_token, data.refresh_token);
  return true;
}
