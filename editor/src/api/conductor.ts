// Conductor REST client — mirrors pkg/conductor/http.go.

import { apiRequest } from "./client";

const BASE = "/api/v1/conductor";

function request<T>(path: string, init?: RequestInit): Promise<T> {
  return apiRequest<T>(BASE + path, init);
}

// Mirror pkg/conductor/state.go Snapshot.
export interface ConductorSnapshot {
  name: string;
  tracker: string;
  generated_at: string;
  polling_interval_seconds: number;
  stall_timeout_seconds: number;
  running: RunningView[] | null;
  retries: RetryView[] | null;
  slots: SlotsView;
}

export interface RunningView {
  issue_id: string;
  identifier: string;
  run_id: string;
  workflow_state: string;
  workspace_path?: string;
  started_at: string;
  last_event_at: string;
  last_event_name?: string;
  attempt?: number;
}

export interface RetryView {
  issue_id: string;
  identifier: string;
  attempt: number;
  due_at: string;
  error?: string;
}

export interface SlotsView {
  global_max: number;
  global_used: number;
  per_state_max?: Record<string, number>;
  per_state_used?: Record<string, number>;
}

export function getState(): Promise<ConductorSnapshot> {
  return request("/state");
}

export function refresh(): Promise<{ queued: boolean }> {
  return request("/refresh", { method: "POST" });
}

export function reload(): Promise<{ reloaded: boolean; polling_interval_s: number }> {
  return request("/reload", { method: "POST" });
}

export function cancelIssue(id: string): Promise<{ status: string }> {
  return request(`/issues/${encodeURIComponent(id)}/cancel`, { method: "POST" });
}

// openWS returns a connected WebSocket that broadcasts Snapshot
// updates. The caller must close it. Reconnection is the caller's
// responsibility.
export function openWS(): WebSocket {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  const url = `${proto}//${window.location.host}${BASE}/ws`;
  return new WebSocket(url);
}
