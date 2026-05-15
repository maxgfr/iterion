// Native kanban tracker — REST client. Mirrors pkg/conductor/native/http.go.
// All paths are relative to the editor's /api base via the shared client.

const BASE = "/api/v1/native";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    credentials: "include",
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(`native API ${res.status}: ${text || res.statusText}`);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

// ---------------------------------------------------------------------------
// Types — mirror pkg/conductor/native/*.go JSON tags
// ---------------------------------------------------------------------------

export interface NativeIssue {
  id: string;
  title: string;
  body?: string;
  state: string;
  labels?: string[];
  priority?: number;
  assignee?: string;
  blockers?: string[];
  fields?: Record<string, unknown>;
  claim?: string;
  created_at: string;
  updated_at: string;
}

export interface NativeBoard {
  states: NativeState[];
  fields?: NativeField[];
  updated_at: string;
}

export interface NativeState {
  name: string;
  display?: string;
  color?: string;
  terminal?: boolean;
  eligible?: boolean;
}

export type NativeFieldType = "text" | "number" | "enum" | "date" | "bool";

export interface NativeField {
  name: string;
  display?: string;
  type: NativeFieldType;
  required?: boolean;
  enum_values?: string[];
  default?: unknown;
}

export interface NativeIssueCreate {
  title: string;
  body?: string;
  state?: string;
  labels?: string[];
  priority?: number;
  assignee?: string;
  blockers?: string[];
  fields?: Record<string, unknown>;
}

export interface NativeIssuePatch {
  title?: string;
  body?: string;
  labels?: string[];
  priority?: number;
  assignee?: string;
  blockers?: string[];
  fields?: Record<string, unknown>;
}

// ---------------------------------------------------------------------------
// REST surface
// ---------------------------------------------------------------------------

export interface ListFilter {
  state?: string[];
  label?: string[];
  assignee?: string;
}

export function listIssues(filter: ListFilter = {}): Promise<NativeIssue[]> {
  const q = new URLSearchParams();
  for (const s of filter.state ?? []) q.append("state", s);
  for (const l of filter.label ?? []) q.append("label", l);
  if (filter.assignee) q.set("assignee", filter.assignee);
  const suffix = q.toString();
  return request(`/issues${suffix ? "?" + suffix : ""}`);
}

export function createIssue(input: NativeIssueCreate): Promise<NativeIssue> {
  return request("/issues", { method: "POST", body: JSON.stringify(input) });
}

export function getIssue(id: string): Promise<NativeIssue> {
  return request(`/issues/${encodeURIComponent(id)}`);
}

export function patchIssue(id: string, patch: NativeIssuePatch): Promise<NativeIssue> {
  return request(`/issues/${encodeURIComponent(id)}`, {
    method: "PATCH",
    body: JSON.stringify(patch),
  });
}

export function deleteIssue(id: string): Promise<void> {
  return request(`/issues/${encodeURIComponent(id)}`, { method: "DELETE" });
}

export function transitionIssue(id: string, to: string): Promise<NativeIssue> {
  return request(`/issues/${encodeURIComponent(id)}/transition`, {
    method: "POST",
    body: JSON.stringify({ to }),
  });
}

export function getBoard(): Promise<NativeBoard> {
  return request("/board");
}

export function putBoard(board: Partial<NativeBoard>): Promise<NativeBoard> {
  return request("/board", { method: "PUT", body: JSON.stringify(board) });
}
