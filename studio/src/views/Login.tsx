import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useLocation } from "wouter";
import { listProviders, register, type ProvidersResponse } from "@/api/auth";
import { ApiError } from "@/api/auth";
import { useAuth } from "@/auth/AuthContext";
import { useServerInfoStore } from "@/store/serverInfo";

const BASE = (import.meta.env.VITE_API_URL ?? "/api").replace(/\/$/, "");

export default function Login() {
  const { signIn, status } = useAuth();
  const [, navigate] = useLocation();
  const serverInfo = useServerInfoStore((s) => s.info);
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [invitation, setInvitation] = useState("");
  const [providers, setProviders] = useState<ProvidersResponse | null>(null);
  const [org, setOrg] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  // Refetch providers as the org slug changes (debounced): the response folds
  // in that org's own SSO providers (its Keycloak) alongside the global ones.
  // An unknown slug returns only the globals, so this is safe to call as typed.
  useEffect(() => {
    const t = setTimeout(() => {
      void listProviders(org.trim() || undefined)
        .then(setProviders)
        .catch(() => setProviders(null));
    }, 250);
    return () => clearTimeout(t);
  }, [org]);

  // Ensure serverInfo is loaded so we can gate the forgot-password link.
  useEffect(() => {
    if (!serverInfo) void useServerInfoStore.getState().refresh();
  }, [serverInfo]);

  useEffect(() => {
    if (status === "authenticated") {
      navigate("/");
    }
  }, [status, navigate]);

  // Pre-fill invitation token from URL.
  useEffect(() => {
    const url = new URL(window.location.href);
    const t = url.searchParams.get("invite");
    if (t) {
      setMode("register");
      setInvitation(t);
    }
  }, []);

  // returnTo captures the in-app URL the user was on before being
  // bounced to /login (typically by AuthGate on session expiry), OR
  // honours an explicit ?next= param (e.g. /invitations/accept bounces
  // here with `?invite=…&next=/invitations/accept?token=…`). We
  // restrict to relative same-origin paths so a hostile `?next=`
  // injection can't open-redirect after login.
  const returnTo = (): string => {
    const u = new URL(window.location.href);
    const next = u.searchParams.get("next");
    if (next && next.startsWith("/") && !next.startsWith("//")) {
      return next;
    }
    const here = window.location.pathname + window.location.search + window.location.hash;
    if (!here || here.startsWith("/login") || here.startsWith("/auth/")) return "/";
    if (!here.startsWith("/") || here.startsWith("//")) return "/";
    return here;
  };

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    setErr(null);
    setBusy(true);
    try {
      if (mode === "login") {
        await signIn(email, password);
      } else {
        await register({ email, password, name, invitation: invitation || undefined });
      }
      navigate(returnTo());
    } catch (e) {
      // 403 "password change required" → forced rotation flow. Carry the
      // email + the rejected password as the temporary credential so the
      // user only has to type the new one.
      if (
        e instanceof ApiError &&
        e.status === 403 &&
        /password change required/i.test(e.message)
      ) {
        const qs = new URLSearchParams({ email, temp: password }).toString();
        navigate(`/auth/password/change?${qs}`, { replace: true });
        return;
      }
      const msg = e instanceof ApiError ? e.message : errorMessage(e);
      setErr(msg);
    } finally {
      setBusy(false);
    }
  };

  const oidcStart = (name: string) => {
    const next = encodeURIComponent(returnTo());
    window.location.href = `${BASE}/auth/oidc/${encodeURIComponent(name)}/start?next=${next}`;
  };

  const showRegister = providers?.signup_mode === "open" || invitation !== "";

  return (
    <div className="min-h-screen flex items-center justify-center bg-surface-0 text-fg-default px-4">
      <div className="w-full max-w-md bg-surface-1 border border-border-subtle rounded-lg p-8 shadow-[var(--shadow-md)]">
        <h1 className="text-2xl font-semibold mb-2">
          {mode === "login" ? "Sign in to iterion" : "Create your account"}
        </h1>
        <p className="text-sm text-fg-muted mb-6">
          {mode === "login"
            ? "Use your team email + password, or one of the SSO providers below."
            : invitation
              ? "You're joining a team via invitation."
              : "Sign up for a new iterion workspace."}
        </p>

        <form onSubmit={submit} className="space-y-3">
          {mode === "register" && (
            <div>
              <label htmlFor="login-name" className="sr-only">
                Name
              </label>
              <Input
                size="md"
                id="login-name"
                placeholder="Name (optional)"
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoComplete="name"
              />
            </div>
          )}
          <div>
            <label htmlFor="email" className="sr-only">
              Email
            </label>
            <Input
              size="md"
              type="email"
              name="email"
              id="email"
              placeholder="Email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              // "username" (not "email") is the token password managers pair with
              // current-password to recognise a login + offer to save it. On
              // register, "email" is the right collect-an-address semantics.
              autoComplete={mode === "login" ? "username" : "email"}
              required
            />
          </div>
          <div>
            <label htmlFor="password" className="sr-only">
              Password
            </label>
            <Input
              size="md"
              type="password"
              name="password"
              id="password"
              placeholder="Password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete={mode === "login" ? "current-password" : "new-password"}
              required
              // Only enforce a minimum when creating a password; a login must
              // accept whatever length the existing password is.
              minLength={mode === "register" ? 8 : undefined}
            />
          </div>
          {mode === "register" && providers?.signup_mode === "invite_only" && (
            <div>
              <label htmlFor="login-invitation" className="sr-only">
                Invitation token
              </label>
              <Input
                size="md"
                id="login-invitation"
                className="font-mono"
                placeholder="Invitation token"
                value={invitation}
                onChange={(e) => setInvitation(e.target.value)}
                required
              />
            </div>
          )}
          {err && (
            <InlineBanner tone="danger" layout="inline">
              {err}
            </InlineBanner>
          )}
          <Button
            variant="primary"
            type="submit"
            loading={busy}
            className="w-full"
          >
            {busy ? "Working…" : mode === "login" ? "Sign in" : "Create account"}
          </Button>
        </form>

        <div className="mt-6 space-y-2">
          <div className="text-xs uppercase tracking-wider text-fg-muted">
            Single sign-on
          </div>
          <label htmlFor="login-org" className="sr-only">
            Organization slug
          </label>
          <Input
            size="md"
            id="login-org"
            placeholder="Organization slug (for your org's SSO)"
            value={org}
            onChange={(e) => setOrg(e.target.value)}
            autoComplete="organization"
          />
          {(providers?.providers?.length ?? 0) > 0 ? (
            <div className="space-y-2">
              {providers!.providers.map((p) => (
                <Button
                  key={p.name}
                  variant="secondary"
                  className="w-full"
                  onClick={() => oidcStart(p.name)}
                >
                  {p.display}
                </Button>
              ))}
            </div>
          ) : (
            org.trim() !== "" && (
              <div className="text-xs text-fg-muted">No SSO providers for that organization.</div>
            )
          )}
        </div>

        <div className="mt-6 text-sm text-fg-muted text-center space-y-1">
          {mode === "login" ? (
            <>
              {showRegister && (
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => setMode("register")}
                  >
                    Need an account? Sign up
                  </Button>
                </div>
              )}
              {serverInfo?.email_enabled && (
                <div>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => navigate("/auth/forgot-password")}
                  >
                    Forgot your password?
                  </Button>
                </div>
              )}
            </>
          ) : (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setMode("login")}
            >
              Already have an account? Sign in
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
