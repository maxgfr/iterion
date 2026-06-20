import { errorMessage } from "@/lib/errorHints";
import { useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { useLocation } from "wouter";

import { requestPasswordReset } from "@/api/auth";

// ForgotPassword fires the reset-mail request. The server is
// anti-enumeration (always 200), so the view shows the same generic
// confirmation regardless of whether the email matched an account.
export default function ForgotPassword() {
  const [, navigate] = useLocation();
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [sent, setSent] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setBusy(true);
    setErr(null);
    try {
      await requestPasswordReset(email);
      setSent(true);
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  if (sent) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-surface-0 text-fg-default px-4">
        <div className="w-full max-w-md bg-surface-1 border border-border-subtle rounded-lg p-8 shadow-md space-y-4">
          <h1 className="text-2xl font-semibold">Check your email</h1>
          <p className="text-sm text-fg-muted">
            If we have an account for that email address, we sent a password-reset link.
            The link expires shortly — open it in the same browser to finish resetting.
          </p>
          <button onClick={() => navigate("/login")} className="underline text-sm">
            Back to sign-in
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-surface-0 text-fg-default px-4">
      <div className="w-full max-w-md bg-surface-1 border border-border-subtle rounded-lg p-8 shadow-md">
        <h1 className="text-2xl font-semibold mb-2">Forgot your password?</h1>
        <p className="text-sm text-fg-muted mb-6">
          Enter your email and we'll send a one-time link to reset it.
        </p>
        <form onSubmit={submit} className="space-y-3">
          <input
            type="email"
            className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
            placeholder="Email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
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
            {busy ? "Sending…" : "Send reset link"}
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
