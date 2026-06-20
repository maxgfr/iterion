import { errorMessage } from "@/lib/errorHints";
import { useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useConfirm } from "@/hooks/useConfirm";
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
                  <div className="text-warning-fg">You are a platform super-admin.</div>
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
      const msg = e instanceof ApiError ? e.message : errorMessage(e);
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
        <div>
          <label htmlFor="settings-current-password" className="sr-only">
            Current password
          </label>
          <Input
            size="md"
            type="password"
            id="settings-current-password"
            placeholder="Current password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            autoComplete="current-password"
            required
          />
        </div>
        <div>
          <label htmlFor="settings-new-password" className="sr-only">
            New password
          </label>
          <Input
            size="md"
            type="password"
            id="settings-new-password"
            placeholder="New password (≥ 8 characters)"
            value={next}
            onChange={(e) => setNext(e.target.value)}
            autoComplete="new-password"
            minLength={8}
            required
          />
        </div>
        <div>
          <label htmlFor="settings-confirm-password" className="sr-only">
            Confirm new password
          </label>
          <Input
            size="md"
            type="password"
            id="settings-confirm-password"
            placeholder="Confirm new password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            autoComplete="new-password"
            minLength={8}
            required
          />
        </div>
        {err && (
          <InlineBanner tone="danger" layout="inline">
            {err}
          </InlineBanner>
        )}
        {ok && (
          <div className="text-sm text-success-fg">Password updated.</div>
        )}
        <Button variant="primary" type="submit" loading={busy}>
          {busy ? "Working…" : "Change password"}
        </Button>
      </form>
    </section>
  );
}

function SignOutEverywhereSection() {
  const { signOut } = useAuth();
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const { confirm, dialog } = useConfirm();

  const submit = async () => {
    const ok = await confirm({
      title: "Sign out everywhere?",
      message: "Revoke every signed-in session for this account?",
      confirmLabel: "Sign out everywhere",
      confirmVariant: "danger",
    });
    if (!ok) return;
    setBusy(true);
    setErr(null);
    try {
      await revokeAllMySessions();
      // The server cleared this window's cookies as well; force the
      // local AuthProvider to flip to anonymous so the UI lands on
      // Login immediately rather than waiting for the next 401.
      await signOut();
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="space-y-3 text-sm bg-surface-1 border border-border-subtle rounded p-4">
      {dialog}
      <div>
        <h3 className="font-medium">Sign out everywhere</h3>
        <p className="text-xs text-fg-subtle mt-0.5">
          Use this when a session may be compromised. PATs are not affected — revoke them on the
          tokens tab.
        </p>
      </div>
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}
      <Button
        variant="danger"
        loading={busy}
        onClick={() => void submit()}
      >
        {busy ? "Working…" : "Sign out everywhere"}
      </Button>
    </section>
  );
}
