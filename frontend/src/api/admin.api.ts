import { api } from "./client";

export type AdminUser = { id: string; name: string; email: string };

const ADMIN_TOKEN_KEY = "pt_admin_session";

export function getAdminToken() {
  return sessionStorage.getItem(ADMIN_TOKEN_KEY);
}

export function setAdminToken(token: string) {
  sessionStorage.setItem(ADMIN_TOKEN_KEY, token);
}

export function clearAdminToken() {
  sessionStorage.removeItem(ADMIN_TOKEN_KEY);
}

function adminHeaders() {
  const token = getAdminToken();
  return token ? { "X-Admin-Session": token } : undefined;
}

export function unlockAdmin(access_code: string) {
  return api<{ admin_token: string; expires_in: number }>("/admin/unlock", {
    method: "POST",
    body: JSON.stringify({ access_code }),
  });
}

export function listAdminUsers() {
  return api<AdminUser[]>("/admin/users", { headers: adminHeaders() });
}

export function adminSetPassword(userId: string, new_password: string) {
  return api<{ ok: boolean }>(`/admin/users/${userId}/password`, {
    method: "POST",
    body: JSON.stringify({ new_password }),
    headers: adminHeaders(),
  });
}
