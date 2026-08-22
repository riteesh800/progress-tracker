import { FormEvent, useState } from "react";
import { useNavigate } from "react-router-dom";
import { forgotPassword, login, register, resetPassword, verifyResetCode } from "../api/auth.api";
import { ApiError } from "../api/client";
import PasswordField from "../components/PasswordField";

type Mode = "login" | "register" | "forgot" | "verify" | "reset";

export default function LoginPage() {
  const nav = useNavigate();
  const [mode, setMode] = useState<Mode>("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [name, setName] = useState("");
  const [code, setCode] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [showConfirm, setShowConfirm] = useState(false);
  const [error, setError] = useState("");
  const [info, setInfo] = useState("");
  const [busy, setBusy] = useState(false);
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

  function switchMode(next: Mode) {
    setMode(next);
    setError("");
    setInfo("");
    setPassword("");
    setConfirm("");
    if (next === "login" || next === "register" || next === "forgot") setCode("");
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError("");
    setInfo("");
    if ((mode === "register" || mode === "reset") && password !== confirm) {
      setError("Passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      if (mode === "login") {
        await login(email, password);
        nav("/");
      } else if (mode === "register") {
        await register({ email, password, name, timezone: tz });
        nav("/");
      } else if (mode === "forgot") {
        const res = await forgotPassword(email);
        setInfo(res.message || "Reset code sent to your email.");
        setMode("verify");
      } else if (mode === "verify") {
        await verifyResetCode(email, code);
        setInfo("Code verified. Choose a new password.");
        setMode("reset");
      } else {
        await resetPassword(email, code, password);
        await login(email, password);
        nav("/");
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong");
    } finally {
      setBusy(false);
    }
  }

  const submitLabel =
    mode === "login"
      ? "Log in"
      : mode === "register"
        ? "Create account"
        : mode === "forgot"
          ? "Send reset code"
          : mode === "verify"
            ? "Verify OTP"
            : "Reset password";

  return (
    <div className="auth-shell">
      <div className="auth-panel">
        <h1>Skill Progress Tracker</h1>
        <p className="muted">Turn a syllabus into a trackable topic tree.</p>
        <form className="card auth-card grid" onSubmit={onSubmit}>
          <label>
            Email
            <input value={email} onChange={(e) => setEmail(e.target.value)} type="email" required autoComplete="email" />
          </label>
          {mode === "register" && (
            <label>
              Name
              <input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" />
            </label>
          )}
          {mode === "verify" && (
            <label>
              Reset code
              <input value={code} onChange={(e) => setCode(e.target.value)} required inputMode="numeric" autoComplete="one-time-code" />
            </label>
          )}
          {(mode === "login" || mode === "register" || mode === "reset") && (
            <PasswordField
              label={mode === "reset" ? "New password" : "Password"}
              value={password}
              onChange={setPassword}
              visible={showPassword}
              onToggle={() => setShowPassword((v) => !v)}
              required
              autoComplete={mode === "login" ? "current-password" : "new-password"}
            />
          )}
          {(mode === "register" || mode === "reset") && (
            <PasswordField
              label="Confirm password"
              value={confirm}
              onChange={setConfirm}
              visible={showConfirm}
              onToggle={() => setShowConfirm((v) => !v)}
              required
              autoComplete="new-password"
            />
          )}
          {error && <div className="error">{error}</div>}
          {info && <div className="settings-ok">{info}</div>}
          <button className={`primary auth-submit${busy ? " is-busy" : ""}`} type="submit" disabled={busy}>
            {busy ? "Please wait…" : submitLabel}
          </button>
          {mode === "register" && <div className="muted">Timezone captured at signup: {tz}</div>}
          <div className="row auth-switch">
            <button type="button" onClick={() => switchMode("login")}>Log in</button>
            <button type="button" onClick={() => switchMode("register")}>Register</button>
            <button type="button" onClick={() => switchMode("forgot")}>Forgot password</button>
          </div>
        </form>
      </div>
    </div>
  );
}
