import { FormEvent, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { forgotPassword, login, me, register, resetPassword, verifyResetCode } from "../api/auth.api";
import { clearAdminToken } from "../api/admin.api";
import { ApiError } from "../api/client";
import PasswordField from "../components/PasswordField";

type Mode = "login" | "register" | "forgot" | "verify" | "reset";

export default function LoginPage() {
  const nav = useNavigate();
  const qc = useQueryClient();
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
  const [isUnregistered, setIsUnregistered] = useState(false);
  const [resendCooldown, setResendCooldown] = useState(0);
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";

  useEffect(() => {
    if (resendCooldown <= 0) return;
    const timer = setInterval(() => {
      setResendCooldown((prev) => Math.max(0, prev - 1));
    }, 1000);
    return () => clearInterval(timer);
  }, [resendCooldown]);

  function switchMode(next: Mode) {
    setMode(next);
    setError("");
    setInfo("");
    setPassword("");
    setConfirm("");
    setIsUnregistered(false);
    if (next === "login" || next === "register" || next === "forgot") {
      setCode("");
    }
  }

  function resetSessionCaches() {
    qc.clear();
    clearAdminToken();
    try {
      sessionStorage.removeItem("pt_ai_draft");
    } catch {
      // ignore storage errors
    }
  }

  async function onResendOtp() {
    if (resendCooldown > 0 || busy || !email) return;
    setError("");
    setInfo("");
    setBusy(true);
    try {
      await forgotPassword(email.trim().toLowerCase());
      setInfo(`A new 6-digit verification code has been sent to ${email.trim()}.`);
      setResendCooldown(60);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to resend code.");
    } finally {
      setBusy(false);
    }
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy) return;
    setError("");
    setInfo("");
    setIsUnregistered(false);

    const cleanEmail = email.trim().toLowerCase();

    // Mode-specific validation & submission
    if (mode === "forgot") {
      if (!/^[^\s@]+@gmail\.com$/i.test(cleanEmail)) {
        setError("Please enter a valid Gmail address (@gmail.com).");
        return;
      }
      setBusy(true);
      try {
        await forgotPassword(cleanEmail);
        setInfo(`Verification code sent to ${cleanEmail}. Please check your inbox.`);
        setResendCooldown(60);
        setMode("verify");
      } catch (err) {
        if (err instanceof ApiError && err.code === "email_not_registered") {
          setIsUnregistered(true);
          setError("This email is not registered with an account.");
        } else {
          setError(err instanceof ApiError ? err.message : "Failed to send reset code.");
        }
      } finally {
        setBusy(false);
      }
      return;
    }

    if (mode === "verify") {
      const cleanCode = code.trim().replace(/\D/g, "");
      if (cleanCode.length !== 6) {
        setError("Please enter the 6-digit verification code sent to your email.");
        return;
      }
      setBusy(true);
      try {
        await verifyResetCode(cleanEmail, cleanCode);
        setInfo("Code verified successfully. Please choose a new password.");
        setMode("reset");
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Invalid or expired reset code.");
      } finally {
        setBusy(false);
      }
      return;
    }

    if (mode === "reset") {
      const cleanCode = code.trim().replace(/\D/g, "");
      if (password.length < 8) {
        setError("Password must be at least 8 characters long.");
        return;
      }
      if (password !== confirm) {
        setError("Passwords do not match.");
        return;
      }
      setBusy(true);
      try {
        await resetPassword(cleanEmail, cleanCode, password);
        resetSessionCaches();
        await login(cleanEmail, password);
        const afterReset = await me();
        nav(afterReset.must_change_password ? "/change-password" : "/");
      } catch (err) {
        setError(err instanceof ApiError ? err.message : "Failed to reset password. Please try again.");
      } finally {
        setBusy(false);
      }
      return;
    }

    if (mode === "register" && password.length < 8) {
      setError("Password must be at least 8 characters long.");
      return;
    }

    if (mode === "register" && password !== confirm) {
      setError("Passwords do not match.");
      return;
    }

    setBusy(true);
    try {
      if (mode === "login") {
        resetSessionCaches();
        await login(cleanEmail, password);
        const u = await me();
        nav(u.must_change_password ? "/change-password" : "/");
      } else if (mode === "register") {
        resetSessionCaches();
        await register({ email: cleanEmail, password, name, timezone: tz });
        nav("/");
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong.");
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
          ? "Send OTP"
          : mode === "verify"
            ? "Verify OTP"
            : "Reset password & Open account";

  return (
    <div className="auth-shell">
      <div className="auth-panel">
        <h1>Skill Progress Tracker</h1>
        <p className="muted">Turn a syllabus into a trackable topic tree.</p>
        <form className="card auth-card grid" onSubmit={onSubmit}>
          {mode === "forgot" && (
            <p className="muted" style={{ margin: 0, fontSize: "0.85rem", lineHeight: 1.45 }}>
              Enter your registered Gmail address below. We will send a 6-digit one-time verification code (OTP) to reset your password.
            </p>
          )}

          {mode === "verify" && (
            <p className="muted" style={{ margin: 0, fontSize: "0.85rem", lineHeight: 1.45 }}>
              A 6-digit verification code has been sent to <strong>{email}</strong>. Enter it below to verify your identity:
            </p>
          )}

          {mode === "reset" && (
            <p className="muted" style={{ margin: 0, fontSize: "0.85rem", lineHeight: 1.45 }}>
              Create a new secure password for <strong>{email}</strong> (minimum 8 characters).
            </p>
          )}

          {(mode === "login" || mode === "register" || mode === "forgot") && (
            <label>
              Email
              <input
                value={email}
                onChange={(e) => {
                  setEmail(e.target.value);
                  setIsUnregistered(false);
                }}
                type="email"
                required
                autoComplete="email"
                pattern="^[^\s@]+@[gG][mM][aA][iI][lL]\.[cC][oO][mM]$"
                placeholder="you@gmail.com"
              />
              {email.length > 0 && !/^[^\s@]+@gmail\.com$/i.test(email) && (
                <div className="error">Enter a valid Gmail address (@gmail.com)</div>
              )}
            </label>
          )}

          {mode === "register" && (
            <label>
              Name
              <input value={name} onChange={(e) => setName(e.target.value)} autoComplete="name" maxLength={25} />
            </label>
          )}

          {mode === "verify" && (
            <label>
              6-digit OTP Code
              <input
                value={code}
                onChange={(e) => setCode(e.target.value.replace(/\D/g, "").slice(0, 6))}
                required
                inputMode="numeric"
                pattern="[0-9]{6}"
                maxLength={6}
                autoComplete="one-time-code"
                placeholder="123456"
                autoFocus
                style={{ textAlign: "center", letterSpacing: "6px", fontSize: "1.25rem", fontWeight: 700 }}
              />
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
              minLength={8}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
            />
          )}

          {(mode === "register" || mode === "reset") && (
            <PasswordField
              label={mode === "reset" ? "Re-enter new password" : "Confirm password"}
              value={confirm}
              onChange={setConfirm}
              visible={showConfirm}
              onToggle={() => setShowConfirm((v) => !v)}
              required
              minLength={8}
              autoComplete="new-password"
            />
          )}

          {error && <div className="error">{error}</div>}
          {info && <div className="settings-ok">{info}</div>}

          {isUnregistered && (
            <div className="auth-unregistered-box">
              <span>This email is not registered yet.</span>
              <button
                type="button"
                className="secondary"
                onClick={() => switchMode("register")}
              >
                Register now with {email || "this email"} →
              </button>
            </div>
          )}

          {mode === "verify" && (
            <div className="row auth-verify-actions" style={{ justifyContent: "space-between", alignItems: "center" }}>
              <button
                type="button"
                className="auth-text-btn"
                onClick={onResendOtp}
                disabled={resendCooldown > 0 || busy}
              >
                {resendCooldown > 0 ? `Resend OTP (${resendCooldown}s)` : "Resend OTP"}
              </button>
              <button
                type="button"
                className="auth-text-btn"
                onClick={() => switchMode("forgot")}
              >
                Change email
              </button>
            </div>
          )}

          <button className={`primary auth-submit${busy ? " is-busy" : ""}`} type="submit" disabled={busy}>
            {busy ? "Please wait…" : submitLabel}
          </button>

          {mode === "register" && <div className="muted">Timezone captured at signup: {tz}</div>}

          <div className="row auth-switch">
            <button
              type="button"
              className={mode === "login" ? "active" : ""}
              onClick={() => switchMode("login")}
            >
              Log in
            </button>
            <button
              type="button"
              className={mode === "register" ? "active" : ""}
              onClick={() => switchMode("register")}
            >
              Register
            </button>
            <button
              type="button"
              className={mode === "forgot" || mode === "verify" || mode === "reset" ? "active" : ""}
              onClick={() => switchMode("forgot")}
            >
              Forgot password
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
