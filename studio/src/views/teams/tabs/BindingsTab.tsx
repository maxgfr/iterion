import { useEffect, useState } from "react";
import { InlineBanner } from "@/components/ui/InlineBanner";

import {
  FeatureUnavailableError,
  type BotSecretBinding,
  createBinding,
  deleteBinding,
  listBindings,
  updateBinding,
} from "@/api/botBindings";
import { type GenericSecretView, listTeamSecrets } from "@/api/secrets";
import { type BotEntryWithSchema, listBots } from "@/api/bots";

import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { EmptyState } from "@/components/ui/EmptyState";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { TagInput } from "@/components/ui/TagInput";
import ConfirmDialog from "@/components/shared/ConfirmDialog";

interface Props {
  teamID: string;
  canManage: boolean;
}

export default function BindingsTab({ teamID, canManage }: Props) {
  const [bots, setBots] = useState<BotEntryWithSchema[]>([]);
  const [secrets, setSecrets] = useState<GenericSecretView[]>([]);
  const [activeBot, setActiveBot] = useState<string>("");
  const [bindings, setBindings] = useState<BotSecretBinding[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [unavailable, setUnavailable] = useState(false);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<BotSecretBinding | null>(null);
  const [deleting, setDeleting] = useState<BotSecretBinding | null>(null);

  // Load bots once.
  useEffect(() => {
    let alive = true;
    void listBots()
      .then((b) => {
        if (!alive) return;
        setBots(b);
        if (b.length > 0 && activeBot === "") setActiveBot(b[0]!.name);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Load team secrets so the create dialog can pick from them.
  useEffect(() => {
    void listTeamSecrets(teamID)
      .then(setSecrets)
      .catch(() => setSecrets([]));
  }, [teamID]);

  const reload = async () => {
    if (!activeBot) return;
    setLoading(true);
    setErr(null);
    try {
      setBindings(await listBindings(teamID, activeBot));
      setUnavailable(false);
    } catch (e) {
      if (e instanceof FeatureUnavailableError) {
        setUnavailable(true);
      } else {
        setErr((e as Error).message);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (activeBot) void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeBot, teamID]);

  const doDelete = async () => {
    if (!deleting) return;
    try {
      await deleteBinding(teamID, deleting.bot_id, deleting.id);
      setDeleting(null);
      void reload();
    } catch (e) {
      setErr((e as Error).message);
    }
  };

  if (unavailable) {
    return (
      <EmptyState
        title="Bot bindings not enabled on this server"
        message="Bot-secret bindings require a multi-tenant deployment."
      />
    );
  }

  return (
    <div className="space-y-4">
      {err && (
        <InlineBanner tone="danger" layout="inline">
          {err}
        </InlineBanner>
      )}

      <div className="flex flex-wrap items-end justify-between gap-2">
        <div>
          <div className="font-medium">Bot secret bindings</div>
          <p className="text-xs text-fg-subtle">
            Map a team secret to the name a bot's workflow declares in its <code>secrets:</code>{" "}
            block, optionally narrowing the egress hosts (ADR-018).
          </p>
        </div>
        <div className="flex items-center gap-2">
          <label className="text-xs text-fg-muted">Bot:</label>
          <Select value={activeBot} onChange={(e) => setActiveBot(e.target.value)}>
            <option value="" disabled>
              — select a bot —
            </option>
            {bots.map((b) => (
              <option key={b.name} value={b.name}>
                {b.display_name ? `${b.display_name} (${b.name})` : b.name}
              </option>
            ))}
          </Select>
          {canManage && activeBot && (
            <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
              Add binding
            </Button>
          )}
        </div>
      </div>

      {!activeBot ? (
        <EmptyState message="Pick a bot to view its bindings." />
      ) : loading ? (
        <EmptyState message="Loading…" />
      ) : bindings.length === 0 ? (
        <EmptyState
          message={
            canManage
              ? "No bindings for this bot. Add one to expose a team secret to the workflow."
              : "No bindings for this bot. Ask an admin to add one."
          }
        />
      ) : (
        <table className="w-full text-sm">
          <thead className="text-xs uppercase tracking-wider text-fg-muted text-left">
            <tr>
              <th className="px-2 py-1">Workflow name</th>
              <th className="px-2 py-1">Secret</th>
              <th className="px-2 py-1">Allowed hosts</th>
              <th className="px-2 py-1">Updated</th>
              <th className="px-2 py-1 text-right">Actions</th>
            </tr>
          </thead>
          <tbody>
            {bindings.map((b) => {
              const sec = secrets.find((s) => s.id === b.secret_id);
              return (
                <tr key={b.id} className="border-t border-border-subtle">
                  <td className="px-2 py-2 font-mono">{b.secret_name_for_workflow}</td>
                  <td className="px-2 py-2">
                    {sec ? (
                      <span>
                        {sec.name}{" "}
                        <span className="text-fg-muted">…{sec.last4 ?? "????"}</span>
                      </span>
                    ) : (
                      <span className="text-danger text-xs">missing ({b.secret_id})</span>
                    )}
                  </td>
                  <td className="px-2 py-2 text-xs">
                    {(b.allowed_hosts ?? []).length === 0 ? (
                      <span className="text-fg-muted">workflow default</span>
                    ) : (
                      (b.allowed_hosts ?? []).map((h) => (
                        <span
                          key={h}
                          className="inline-block bg-surface-2 rounded px-1 mr-1"
                        >
                          {h}
                        </span>
                      ))
                    )}
                  </td>
                  <td className="px-2 py-2 text-fg-muted text-xs">
                    {new Date(b.updated_at).toLocaleString()}
                  </td>
                  <td className="px-2 py-2 text-right space-x-1 whitespace-nowrap">
                    {canManage && (
                      <>
                        <Button size="sm" variant="ghost" onClick={() => setEditing(b)}>
                          Edit
                        </Button>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="text-danger"
                          onClick={() => setDeleting(b)}
                        >
                          Delete
                        </Button>
                      </>
                    )}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}

      {(creating || editing) && activeBot && (
        <BindingDialog
          teamID={teamID}
          botID={activeBot}
          secrets={secrets}
          initial={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
          onSaved={() => {
            setCreating(false);
            setEditing(null);
            void reload();
          }}
        />
      )}

      <ConfirmDialog
        open={deleting !== null}
        title="Delete binding?"
        message="The bot's workflow will no longer resolve this name. Workflows that rely on it will fail until you re-bind it."
        confirmLabel="Delete"
        confirmVariant="danger"
        onConfirm={() => void doDelete()}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

function BindingDialog({
  teamID,
  botID,
  secrets,
  initial,
  onClose,
  onSaved,
}: {
  teamID: string;
  botID: string;
  secrets: GenericSecretView[];
  initial: BotSecretBinding | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [secretID, setSecretID] = useState(initial?.secret_id ?? "");
  const [name, setName] = useState(initial?.secret_name_for_workflow ?? "");
  const [hosts, setHosts] = useState<string[]>(initial?.allowed_hosts ?? []);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const submit = async () => {
    setBusy(true);
    setErr(null);
    try {
      if (initial) {
        await updateBinding(teamID, botID, initial.id, {
          secret_id: secretID,
          secret_name_for_workflow: name,
          allowed_hosts: hosts,
        });
      } else {
        await createBinding(teamID, botID, {
          secret_id: secretID,
          secret_name_for_workflow: name,
          allowed_hosts: hosts.length ? hosts : undefined,
        });
      }
      onSaved();
    } catch (e) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const valid = secretID !== "" && name.trim() !== "";

  return (
    <Dialog
      open
      onOpenChange={(v) => {
        if (!v) onClose();
      }}
      title={initial ? "Edit binding" : "New binding"}
      widthClass="max-w-lg"
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" disabled={!valid} loading={busy} onClick={() => void submit()}>
            {initial ? "Save" : "Create"}
          </Button>
        </>
      }
    >
      {err && (
        <InlineBanner tone="danger" layout="inline" className="mb-3">
          {err}
        </InlineBanner>
      )}
      <div className="space-y-3 text-sm">
        <label className="block">
          <div className="text-xs text-fg-muted mb-1">Team secret</div>
          <Select value={secretID} onChange={(e) => setSecretID(e.target.value)}>
            <option value="">— pick a secret —</option>
            {secrets.map((s) => (
              <option key={s.id} value={s.id}>
                {s.name} (…{s.last4 ?? "????"})
              </option>
            ))}
          </Select>
        </label>
        <label className="block">
          <div className="text-xs text-fg-muted mb-1">Workflow name</div>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="forge_token"
          />
          <div className="text-[10px] text-fg-subtle mt-1">
            The name the bot's workflow declares in its <code>secrets:</code> block. Must match
            exactly.
          </div>
        </label>
        <label className="block">
          <div className="text-xs text-fg-muted mb-1">Allowed egress hosts (optional)</div>
          <TagInput value={hosts} onChange={setHosts} placeholder="gitlab.example.com" />
          <div className="text-[10px] text-fg-subtle mt-1">
            ADR-018: if set, these hosts intersect (never broaden) the workflow's declared{" "}
            <code>hosts:</code>. Leave empty to keep the workflow default.
          </div>
        </label>
      </div>
    </Dialog>
  );
}
