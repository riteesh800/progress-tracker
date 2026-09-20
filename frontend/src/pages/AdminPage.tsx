import { FormEvent, useEffect, useState } from "react";
import { Navigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { me } from "../api/auth.api";
import { getAccess } from "../api/client";
import { ApiError } from "../api/client";
import {
  adminSetPassword,
  clearAdminToken,
  getAdminToken,
  listAdminUsers,
  setAdminToken,
  unlockAdmin,
} from "../api/admin.api";
import PasswordField from "../components/PasswordField";

export default function AdminPage() {
  const profile = useQuery({ queryKey: ["me"], queryFn: me, enabled: Boolean(getAccess()) });
  const [code, setCode] = useState("");
  const [codeVisible, setCodeVisible] = useState(false);
  const [unlockError, setUnlockError] = useState("");
  const [unlocking, setUnlocking] = useState(false);
  const [unlocked, setUnlocked] = useState(Boolean(getAdminToken()));

  const [target, setTarget] = useState<{ id: string; email: string; name: string } | null>(null);
  const [newPassword, setNewPassword] = useState("");
  const [pwVisible, setPwVisible] = useState(false);
  const [pwError, setPwError] = useState("");
  const [pwOk, setPwOk] = useState("");
  const [savingPw, setSavingPw] = useState(false);

  const users = useQuery({
    queryKey: ["admin-users"],
    queryFn: listAdminUsers,
    enabled: unlocked && Boolean(profile.data?.can_open_admin),
    retry: false,
  });

  useEffect(() => {
    if (!users.isError) return;
    const err = users.error;
    if (err instanceof ApiError && (err.code === "admin_locked" || err.status === 401)) {
      clearAdminToken();
      setUnlocked(false);
    }
  }, [users.isError, users.error]);

  if (profile.isLoading) return <p>Loading…</p>;
  if (profile.isError || !profile.data) return <p className="error">Could not load profile.</p>;
  if (!profile.data.can_open_admin) return <Navigate to="/settings" replace />;

  async function onUnlock(e: FormEvent) {
    e.preventDefault();
    setUnlockError("");
    setUnlocking(true);
    try {
      const res = await unlockAdmin(code);
      setAdminToken(res.admin_token);
      setCode("");
      setUnlocked(true);
    } catch (err) {
      clearAdminToken();
      setUnlocked(false);
      setUnlockError(err instanceof ApiError ? err.message : "Could not verify the Database Access Code.");
    } finally {
      setUnlocking(false);
    }
  }

  async function onChangePassword(e: FormEvent) {
    e.preventDefault();
    if (!target) return;
    setPwError("");
    setPwOk("");
    if (newPassword.length < 8) {
      setPwError("Password must be at least 8 characters.");
      return;
    }
    setSavingPw(true);
    try {
      await adminSetPassword(target.id, newPassword);
      setPwOk(`Password updated for ${target.email}.`);
      setNewPassword("");
      setTarget(null);
    } catch (err) {
      if (err instanceof ApiError && (err.code === "admin_locked" || err.status === 401)) {
        clearAdminToken();
        setUnlocked(false);
        setPwError("Admin session expired. Enter the Database Access Code again.");
      } else {
        setPwError(err instanceof ApiError ? err.message : "Could not change password.");
      }
    } finally {
      setSavingPw(false);
    }
  }

  if (!unlocked) {
    return (
      <div className="grid admin-page">
        <h1>Admin</h1>
        <p className="muted">Enter the Database Access Code to continue. This is not your account password.</p>
        <form className="card admin-unlock" onSubmit={onUnlock}>
          <PasswordField
            label="Database Access Code"
            value={code}
            onChange={setCode}
            visible={codeVisible}
            onToggle={() => setCodeVisible((v) => !v)}
            required
            minLength={12}
            autoComplete="off"
          />
          {unlockError && <p className="error">{unlockError}</p>}
          <button className="primary" type="submit" disabled={unlocking}>
            {unlocking ? "Verifying…" : "Verify"}
          </button>
        </form>
      </div>
    );
  }

  return (
    <div className="grid admin-page">
      <h1>Admin</h1>
      <p className="muted">Current accounts in the database. New sign-ups appear here; deleted accounts disappear.</p>
      {pwOk && <p className="settings-ok">{pwOk}</p>}
      {users.isLoading && <p>Loading accounts…</p>}
      {users.isError && <p className="error">Could not load accounts. Try verifying the access code again.</p>}
      {users.data && (
        <div className="card admin-table-wrap">
          <div className="admin-table-head">
            <span>Name</span>
            <span>Email</span>
            <span>Change Password</span>
          </div>
          {users.data.map((u) => (
            <div className="admin-table-row" key={u.id}>
              <span title={u.name}>{u.name || "—"}</span>
              <span title={u.email}>{u.email}</span>
              <span>
                <button type="button" onClick={() => { setTarget(u); setPwError(""); setNewPassword(""); }}>
                  Change Password
                </button>
              </span>
            </div>
          ))}
        </div>
      )}

      {target && (
        <form className="card admin-pw-card" onSubmit={onChangePassword}>
          <h2>Change password</h2>
          <p className="muted">
            Set a new password for <strong>{target.name || target.email}</strong> ({target.email}). This does not
            change any other account.
          </p>
          <PasswordField
            label="New password"
            value={newPassword}
            onChange={setNewPassword}
            visible={pwVisible}
            onToggle={() => setPwVisible((v) => !v)}
            required
            minLength={8}
            autoComplete="new-password"
          />
          {pwError && <p className="error">{pwError}</p>}
          <div className="row">
            <button type="button" onClick={() => !savingPw && setTarget(null)}>Cancel</button>
            <button className="primary" type="submit" disabled={savingPw}>
              {savingPw ? "Saving…" : "Save password"}
            </button>
          </div>
        </form>
      )}
    </div>
  );
}
