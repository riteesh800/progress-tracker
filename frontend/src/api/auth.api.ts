import { api, setTokens, clearTokens, getRefresh } from "./client";

export type TokenPair = { access_token: string; refresh_token: string; token_type: string };
export type User = { id: string; email: string; name: string; timezone: string; is_active: boolean };

export async function register(body: { email: string; password: string; name: string; timezone: string }) {
  const tokens = await api<TokenPair>("/auth/register", { method: "POST", body: JSON.stringify(body), auth: false });
  setTokens(tokens.access_token, tokens.refresh_token);
  return tokens;
}
export async function login(email: string, password: string) {
  const tokens = await api<TokenPair>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
    auth: false,
  });
  setTokens(tokens.access_token, tokens.refresh_token);
  return tokens;
}
export async function googleLogin(id_token: string, timezone: string) {
  const tokens = await api<TokenPair>("/auth/google", {
    method: "POST",
    body: JSON.stringify({ id_token, timezone }),
    auth: false,
  });
  setTokens(tokens.access_token, tokens.refresh_token);
  return tokens;
}
export async function me() {
  return api<User>("/auth/me");
}
export async function updateMe(body: Partial<Pick<User, "name" | "timezone">>) {
  return api<User>("/auth/me", { method: "PATCH", body: JSON.stringify(body) });
}
export async function logout() {
  const refresh = getRefresh();
  if (refresh) {
    try {
      await api("/auth/logout", { method: "POST", body: JSON.stringify({ refresh_token: refresh }) });
    } catch {
      /* still clear locally */
    }
  }
  clearTokens();
}
export async function forgotPassword(email: string) {
  return api<{ ok: boolean; message?: string }>("/auth/forgot-password", {
    method: "POST",
    body: JSON.stringify({ email }),
    auth: false,
  });
}
export async function verifyResetCode(email: string, code: string) {
  return api<{ ok: boolean }>("/auth/verify-reset-code", {
    method: "POST",
    body: JSON.stringify({ email, code }),
    auth: false,
  });
}
export async function resetPassword(email: string, code: string, new_password: string) {
  return api("/auth/reset-password", {
    method: "POST",
    body: JSON.stringify({ email, code, new_password }),
    auth: false,
  });
}
export async function deleteAccount() {
  await api("/auth/me", { method: "DELETE" });
  clearTokens();
}
