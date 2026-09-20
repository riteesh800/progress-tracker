import { FormEvent, useState } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { changeOwnPassword, me } from "../api/auth.api";
import { ApiError, getAccess } from "../api/client";
import PasswordField from "../components/PasswordField";

export default function ForceChangePasswordPage() {
  const nav = useNavigate();
  const qc = useQueryClient();
  const profile = useQuery({ queryKey: ["me"], queryFn: me, enabled: Boolean(getAccess()), retry: false });
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (!getAccess()) return <Navigate to="/login" replace />;
  if (profile.isLoading) return <p>Loading…</p>;
  if (profile.isError) return <Navigate to="/login" replace />;
  if (profile.data && !profile.data.must_change_password) return <Navigate to="/" replace />;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError("");
    if (password !== confirm) {
      setError("Passwords do not match.");
      return;
    }
    if (password.length < 8) {
      setError("Password must be at least 8 characters.");
      return;
    }
    setBusy(true);
    try {
      await changeOwnPassword(password);
      await qc.invalidateQueries({ queryKey: ["me"] });
      nav("/", { replace: true });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Could not update password.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="auth-shell">
      <div className="auth-panel">
        <h1>Reset your password</h1>
        <p className="muted">An admin set a temporary password. Choose a new password to enter your account.</p>
        <form className="card auth-card grid" onSubmit={onSubmit}>
          <PasswordField
            label="New password"
            value={password}
            onChange={setPassword}
            visible={showPassword}
            onToggle={() => setShowPassword((v) => !v)}
            required
            minLength={8}
            autoComplete="new-password"
          />
          <PasswordField
            label="Confirm password"
            value={confirm}
            onChange={setConfirm}
            visible={showConfirm}
            onToggle={() => setShowConfirm((v) => !v)}
            required
            minLength={8}
            autoComplete="new-password"
          />
          {error && <div className="error">{error}</div>}
          <button className={`primary auth-submit${busy ? " is-busy" : ""}`} type="submit" disabled={busy}>
            {busy ? "Saving…" : "Save password and continue"}
          </button>
        </form>
      </div>
    </div>
  );
}
