import { useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useLocation } from "wouter";

import { ApiError, completePendingPasswordChange } from "@/api/auth";
import { useAuth } from "@/auth/AuthContext";

// ForcedPasswordChange completes the pending_password_change flow for an
// account (typically the bootstrapped super-admin) whose login was
// rejected by /api/auth/login with HTTP 403 "password change required".
// The Login view navigates here carrying the email + the rejected
// password as query params so the user only types the new password.
export default function ForcedPasswordChange() {
  const { reloadIdentity } = useAuth();
  const [, navigate] = useLocation();

  const [email, setEmail] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Pull email + temp from the URL so the operator's bootstrap mail
  // (or the Login view's redirect) just works. The history entry is
  // replaced immediately to avoid leaving the temp password in the
  // browser's URL bar / back stack.
  useEffect(() => {
    const u = new URL(window.location.href);
    const e = u.searchParams.get("email") ?? "";
    const p = u.searchParams.get("temp") ?? "";
    if (e) setEmail(e);
    if (p) setCurrentPassword(p);
    if (e || p) {
      const clean = window.location.pathname;
      window.history.replaceState({}, "", clean);
    }
  }, []);

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setErr(null);
    if (newPassword.length < 8) {
      setErr("New password must be at least 8 characters.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setErr("The two new-password fields don't match.");
      return;
    }
    setBusy(true);
    try {
      await completePendingPasswordChange(email, currentPassword, newPassword);
      await reloadIdentity();
      navigate("/", { replace: true });
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : (e as Error).message;
      setErr(msg);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-surface-0 text-fg-default px-4">
      <div className="w-full max-w-md bg-surface-1 border border-border-subtle rounded-lg p-8 shadow-md">
        <h1 className="text-2xl font-semibold mb-2">Choose a new password</h1>
        <p className="text-sm text-fg-muted mb-6">
          Your account was created with a temporary password. Set a new one to finish signing in.
        </p>
        <form onSubmit={submit} className="space-y-3" data-testid="forced-password-change-form">
          <input
            type="email"
            className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            autoComplete="email"
            required
          />
          <input
            type="password"
            className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
            placeholder="Current (temporary) password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            autoComplete="current-password"
            required
          />
          <input
            type="password"
            className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
            placeholder="New password (≥ 8 characters)"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            autoComplete="new-password"
            minLength={8}
            required
          />
          <input
            type="password"
            className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
            placeholder="Confirm new password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            autoComplete="new-password"
            minLength={8}
            required
          />
          {err && (
            <InlineBanner tone="danger" layout="inline">
              {err}
            </InlineBanner>
          )}
          <button
            type="submit"
            disabled={busy}
            className="w-full bg-accent text-fg-onAccent rounded px-3 py-2 font-medium disabled:opacity-50"
          >
            {busy ? "Working…" : "Set new password & sign in"}
          </button>
        </form>
        <div className="mt-4 text-sm text-fg-muted text-center">
          <button onClick={() => navigate("/login")} className="underline">
            Back to sign-in
          </button>
        </div>
      </div>
    </div>
  );
}
