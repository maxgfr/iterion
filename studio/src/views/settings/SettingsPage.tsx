import { useState } from "react";
import ApiKeysPanel from "./ApiKeys";
import OAuthConnections from "./OAuthConnections";
import TokensPanel from "./TokensPanel";
import { ApiError, changeMyPassword, revokeAllMySessions } from "@/api/auth";
import { useAuth } from "@/auth/AuthContext";
import { useServerInfoStore } from "@/store/serverInfo";
import { useHeaderSlot } from "@/components/shared/useHeaderSlot";

type Tab = "api-keys" | "oauth" | "tokens" | "profile";

export default function SettingsPage() {
  const { user } = useAuth();
  const serverInfo = useServerInfoStore((s) => s.info);
  const [tab, setTab] = useState<Tab>("api-keys");

  useHeaderSlot({
    left: <span className="text-sm font-semibold">Settings</span>,
  });

  const showAuthTabs = serverInfo?.auth_required !== false;

  const tabs: Array<{ id: Tab; label: string }> = [
    { id: "api-keys", label: "API keys (BYOK)" },
    { id: "oauth", label: "OAuth subscriptions" },
  ];
  if (showAuthTabs) tabs.push({ id: "tokens", label: "Access tokens" });
  tabs.push({ id: "profile", label: "Profile" });

  return (
    <div className="h-full overflow-auto">
      <div className="max-w-5xl mx-auto p-3 sm:p-6 grid grid-cols-1 sm:grid-cols-[200px,1fr] gap-4 sm:gap-6">
        <nav className="flex sm:block sm:space-y-1 gap-1 flex-wrap">
          {tabs.map((t) => (
            <button
              key={t.id}
              onClick={() => setTab(t.id)}
              className={`sm:w-full text-left px-3 py-2 rounded text-sm min-h-[44px] sm:min-h-0 ${
                tab === t.id ? "bg-surface-2" : "hover:bg-surface-1"
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>

        <div className="bg-surface-0">
          {tab === "api-keys" && <ApiKeysPanel />}
          {tab === "oauth" && <OAuthConnections />}
          {tab === "tokens" && showAuthTabs && <TokensPanel />}
          {tab === "profile" && (
            <div className="space-y-6">
              <section className="space-y-3 text-sm">
                <h2 className="text-lg font-semibold">Profile</h2>
                <div>Email: {user?.email}</div>
                {user?.name && <div>Name: {user.name}</div>}
                <div>Status: {user?.status}</div>
                {user?.is_super_admin && (
                  <div className="text-fg-warn">You are a platform super-admin.</div>
                )}
              </section>
              {showAuthTabs && <ChangePasswordSection />}
              {showAuthTabs && <SignOutEverywhereSection />}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function ChangePasswordSection() {
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [ok, setOk] = useState(false);

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setErr(null);
    setOk(false);
    if (next.length < 8) {
      setErr("New password must be at least 8 characters.");
      return;
    }
    if (next !== confirm) {
      setErr("The two new-password fields don't match.");
      return;
    }
    setBusy(true);
    try {
      await changeMyPassword(current, next);
      setOk(true);
      setCurrent("");
      setNext("");
      setConfirm("");
    } catch (e) {
      const msg = e instanceof ApiError ? e.message : (e as Error).message;
      setErr(msg);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="space-y-3 text-sm bg-surface-1 border border-border-subtle rounded p-4">
      <div>
        <h3 className="font-medium">Change password</h3>
        <p className="text-xs text-fg-subtle mt-0.5">
          Other sessions will be revoked. This window continues with a fresh cookie.
        </p>
      </div>
      <form onSubmit={submit} className="space-y-2 max-w-md">
        <input
          type="password"
          className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
          placeholder="Current password"
          value={current}
          onChange={(e) => setCurrent(e.target.value)}
          autoComplete="current-password"
          required
        />
        <input
          type="password"
          className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
          placeholder="New password (≥ 8 characters)"
          value={next}
          onChange={(e) => setNext(e.target.value)}
          autoComplete="new-password"
          minLength={8}
          required
        />
        <input
          type="password"
          className="w-full bg-surface-0 border border-border-subtle rounded px-3 py-2"
          placeholder="Confirm new password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          autoComplete="new-password"
          minLength={8}
          required
        />
        {err && (
          <div className="text-sm text-fg-error bg-surface-warn-subtle border border-border-warn rounded px-3 py-2">
            {err}
          </div>
        )}
        {ok && (
          <div className="text-sm text-fg-success">Password updated.</div>
        )}
        <button
          type="submit"
          disabled={busy}
          className="bg-fg-accent text-surface-0 rounded px-3 py-2 text-sm disabled:opacity-50"
        >
          {busy ? "Working…" : "Change password"}
        </button>
      </form>
    </section>
  );
}

function SignOutEverywhereSection() {
  const { signOut } = useAuth();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    if (!confirm("Revoke every signed-in session for this account?")) return;
    setBusy(true);
    setErr(null);
    try {
      await revokeAllMySessions();
      // The server cleared this window's cookies as well; force the
      // local AuthProvider to flip to anonymous so the UI lands on
      // Login immediately rather than waiting for the next 401.
      await signOut();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="space-y-3 text-sm bg-surface-1 border border-border-subtle rounded p-4">
      <div>
        <h3 className="font-medium">Sign out everywhere</h3>
        <p className="text-xs text-fg-subtle mt-0.5">
          Use this when a session may be compromised. PATs are not affected — revoke them on the
          tokens tab.
        </p>
      </div>
      {err && (
        <div className="text-sm text-fg-error bg-surface-warn-subtle border border-border-warn rounded px-3 py-2">
          {err}
        </div>
      )}
      <button
        type="button"
        disabled={busy}
        onClick={() => void submit()}
        className="bg-danger text-fg-onAccent rounded px-3 py-2 text-sm disabled:opacity-50"
      >
        {busy ? "Working…" : "Sign out everywhere"}
      </button>
    </section>
  );
}
