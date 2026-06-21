import { errorMessage } from "@/lib/errorHints";
import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/Badge";
import { InlineBanner } from "@/components/ui/InlineBanner";
import { Button } from "@/components/ui/Button";
import { Textarea } from "@/components/ui/Textarea";
import { useConfirm } from "@/hooks/useConfirm";
import { EmptyState } from "@/components/ui/EmptyState";
import {
  type OAuthConnection,
  type OAuthKind,
  deleteOAuth,
  listOAuthConnections,
  refreshOAuth,
  uploadOAuthCredentials,
} from "@/api/byok";

const KINDS: Array<{
  kind: OAuthKind;
  display: string;
  filename: string;
  hint: string;
}> = [
  {
    kind: "claude_code",
    display: "Claude Code",
    filename: "~/.claude/.credentials.json",
    hint: "Paste the contents of ~/.claude/.credentials.json from a machine where you've signed into Claude Code with your Pro/Max subscription.",
  },
  {
    kind: "codex",
    display: "OpenAI Codex",
    filename: "~/.codex/auth.json",
    hint: "Paste the contents of ~/.codex/auth.json from a machine where you've signed into Codex with your ChatGPT subscription.",
  },
];

export default function OAuthConnections() {
  const [conns, setConns] = useState<OAuthConnection[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [editingKind, setEditingKind] = useState<OAuthKind | null>(null);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const { confirm, dialog } = useConfirm();

  const reload = async () => {
    setLoading(true);
    setErr(null);
    try {
      setConns(await listOAuthConnections());
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
  }, []);

  const submit = async (ev: React.FormEvent) => {
    ev.preventDefault();
    if (!editingKind) return;
    setBusy(true);
    setErr(null);
    try {
      await uploadOAuthCredentials(editingKind, draft);
      setEditingKind(null);
      setDraft("");
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const refresh = async (kind: OAuthKind) => {
    setBusy(true);
    setErr(null);
    try {
      await refreshOAuth(kind);
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (kind: OAuthKind) => {
    const ok = await confirm({
      title: `Disconnect ${kind}?`,
      message: `You'll need to re-paste the credentials to reconnect.`,
      confirmLabel: "Disconnect",
      confirmVariant: "danger",
    });
    if (!ok) return;
    try {
      await deleteOAuth(kind);
      void reload();
    } catch (e) {
      setErr(errorMessage(e));
    }
  };

  const lookup = (kind: OAuthKind) => conns.find((c) => c.kind === kind);

  return (
    <div className="space-y-4">
      {dialog}
      <div>
        <h2 className="text-lg font-semibold">OAuth subscriptions (forfait)</h2>
        <p className="text-sm text-fg-muted mt-1">
          Connect your personal Claude Pro/Max or ChatGPT subscription so iterion can run agents
          on your behalf via the official Claude Code / Codex CLIs. The blob is sealed at rest;
          iterion never reuses your forfait credentials in its in-process LLM client (CGU).
        </p>
      </div>

      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}

      {loading ? (
        <EmptyState message="Loading…" />
      ) : (
        <div className="space-y-4">
          {KINDS.map(({ kind, display, filename, hint }) => {
            const conn = lookup(kind);
            const expiring = conn?.access_token_expires_at
              ? new Date(conn.access_token_expires_at).getTime() - Date.now() < 24 * 3600_000
              : false;
            return (
              <div
                key={kind}
                className="bg-surface-1 border border-border-subtle rounded p-4 space-y-3"
              >
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="font-medium">{display}</h3>
                    <div className="text-xs text-fg-muted">{filename}</div>
                  </div>
                  <div className="text-sm">
                    {conn ? (
                      <Badge variant={expiring ? "warning" : "success"}>
                        Connected
                        {conn.access_token_expires_at &&
                          ` · expires ${new Date(conn.access_token_expires_at).toLocaleString()}`}
                      </Badge>
                    ) : (
                      <Badge variant="neutral">Not connected</Badge>
                    )}
                  </div>
                </div>

                {editingKind === kind ? (
                  <form onSubmit={submit} className="space-y-2">
                    <label
                      htmlFor={`oauth-creds-${kind}`}
                      className="block text-xs text-fg-muted"
                    >
                      {hint}
                    </label>
                    <Textarea
                      id={`oauth-creds-${kind}`}
                      className="font-mono text-xs"
                      rows={6}
                      placeholder='{ "claudeAiOauth": { "accessToken": "...", … } }'
                      value={draft}
                      onChange={(e) => setDraft(e.target.value)}
                      required
                    />
                    <div className="flex gap-2">
                      <Button
                        variant="primary"
                        type="submit"
                        loading={busy}
                      >
                        {busy ? "Sealing…" : "Save"}
                      </Button>
                      <Button
                        variant="secondary"
                        onClick={() => {
                          setEditingKind(null);
                          setDraft("");
                        }}
                      >
                        Cancel
                      </Button>
                    </div>
                  </form>
                ) : (
                  <div className="flex gap-2">
                    <Button
                      variant="primary"
                      onClick={() => {
                        setEditingKind(kind);
                        setDraft("");
                      }}
                    >
                      {conn ? "Update credentials" : "Connect"}
                    </Button>
                    {conn && (
                      <>
                        <Button
                          variant="secondary"
                          onClick={() => refresh(kind)}
                          disabled={busy}
                        >
                          Refresh tokens
                        </Button>
                        <Button
                          variant="danger"
                          onClick={() => remove(kind)}
                        >
                          Disconnect
                        </Button>
                      </>
                    )}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
