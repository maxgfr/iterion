import { useState } from "react";
import ApiKeysPanel from "./ApiKeys";
import OAuthConnections from "./OAuthConnections";
import { useAuth } from "@/auth/AuthContext";
import AppHeader from "@/components/shared/AppHeader";

type Tab = "api-keys" | "oauth" | "profile";

export default function SettingsPage() {
  const { user } = useAuth();
  const [tab, setTab] = useState<Tab>("api-keys");

  return (
    <div className="min-h-screen bg-surface-0 text-fg-default">
      <AppHeader showBackendPill={false}>
        <span className="text-sm font-semibold">Settings</span>
      </AppHeader>

      <div className="max-w-5xl mx-auto p-3 sm:p-6 grid grid-cols-1 sm:grid-cols-[200px,1fr] gap-4 sm:gap-6">
        <nav className="flex sm:block sm:space-y-1 gap-1 flex-wrap">
          {(
            [
              { id: "api-keys", label: "API keys (BYOK)" },
              { id: "oauth", label: "OAuth subscriptions" },
              { id: "profile", label: "Profile" },
            ] as Array<{ id: Tab; label: string }>
          ).map((t) => (
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

        <main className="bg-surface-0">
          {tab === "api-keys" && <ApiKeysPanel />}
          {tab === "oauth" && <OAuthConnections />}
          {tab === "profile" && (
            <div className="space-y-3 text-sm">
              <h2 className="text-lg font-semibold">Profile</h2>
              <div>Email: {user?.email}</div>
              {user?.name && <div>Name: {user.name}</div>}
              <div>Status: {user?.status}</div>
              {user?.is_super_admin && (
                <div className="text-fg-warn">You are a platform super-admin.</div>
              )}
            </div>
          )}
        </main>
      </div>
    </div>
  );
}
