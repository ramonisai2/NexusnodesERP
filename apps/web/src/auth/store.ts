import { create } from "zustand";
import type { SessionClaims } from "./policy";

const TOKEN_KEY = "nexus_access_token";
const OPERATOR_KEY = "nexus_operator_label";
const SESSION_KEY = "nexus_session_id";
const STATION_KEY = "nexus_station_id";

type AuthState = {
  token: string | null;
  claims: SessionClaims | null;
  activeBranchId: string | null;
  operatorLabel: string | null;
  sessionId: string | null;
  stationId: string | null;
  setSession: (token: string, claims: SessionClaims, extras?: { operatorLabel?: string; sessionId?: string; stationId?: string }) => void;
  setActiveBranch: (branchId: string) => void;
  logout: () => void;
  hydrate: () => Promise<void>;
};

async function fetchClaims(token: string): Promise<SessionClaims> {
  const res = await fetch("/api/me", {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) throw new Error("session_invalid");
  return res.json();
}

export const useAuthStore = create<AuthState>((set, get) => ({
  token: null,
  claims: null,
  activeBranchId: null,
  operatorLabel: null,
  sessionId: null,
  stationId: null,
  setSession: (token, claims, extras) => {
    sessionStorage.setItem(TOKEN_KEY, token);
    const operatorLabel =
      extras?.operatorLabel ?? claims.operator_label ?? (claims.attrs?.operator_label as string | undefined) ?? null;
    const sessionId =
      extras?.sessionId ?? claims.session_id ?? (claims.attrs?.session_id as string | undefined) ?? claims.sid ?? null;
    const stationId = extras?.stationId ?? (claims.attrs?.station_id as string | undefined) ?? null;
    if (operatorLabel) sessionStorage.setItem(OPERATOR_KEY, operatorLabel);
    else sessionStorage.removeItem(OPERATOR_KEY);
    if (sessionId) sessionStorage.setItem(SESSION_KEY, sessionId);
    else sessionStorage.removeItem(SESSION_KEY);
    if (stationId) sessionStorage.setItem(STATION_KEY, stationId);
    else sessionStorage.removeItem(STATION_KEY);
    set({
      token,
      claims: { ...claims, operator_label: operatorLabel ?? undefined, session_id: sessionId ?? undefined },
      activeBranchId: claims.branch_ids[0] ?? null,
      operatorLabel,
      sessionId,
      stationId,
    });
  },
  setActiveBranch: (branchId) => set({ activeBranchId: branchId }),
  logout: () => {
    sessionStorage.removeItem(TOKEN_KEY);
    sessionStorage.removeItem(OPERATOR_KEY);
    sessionStorage.removeItem(SESSION_KEY);
    sessionStorage.removeItem(STATION_KEY);
    set({ token: null, claims: null, activeBranchId: null, operatorLabel: null, sessionId: null, stationId: null });
  },
  hydrate: async () => {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (!token) return;
    try {
      const claims = await fetchClaims(token);
      get().setSession(token, claims, {
        operatorLabel: sessionStorage.getItem(OPERATOR_KEY) ?? undefined,
        sessionId: sessionStorage.getItem(SESSION_KEY) ?? undefined,
        stationId: sessionStorage.getItem(STATION_KEY) ?? undefined,
      });
    } catch {
      get().logout();
    }
  },
}));

export type DevPersona = "analyst" | "approver" | "wh_manager" | "regional" | "owner";

export async function loginWithDevToken(
  persona: DevPersona = "analyst",
  opts?: { operatorLabel?: string; stationId?: string },
): Promise<void> {
  const qs = new URLSearchParams({ persona });
  if (opts?.operatorLabel) qs.set("operator_label", opts.operatorLabel);
  if (opts?.stationId) qs.set("station_id", opts.stationId);
  const res = await fetch(`/api/auth/dev-token?${qs.toString()}`, { method: "POST" });
  if (!res.ok) throw new Error("dev_token_failed");
  const data = await res.json();
  useAuthStore.getState().setSession(data.access_token, data.claims, {
    operatorLabel: data.station?.operator_label ?? opts?.operatorLabel,
    sessionId: data.station?.id ?? data.claims?.session_id,
    stationId: data.station?.station_id ?? opts?.stationId,
  });
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const { token, activeBranchId, operatorLabel, sessionId } = useAuthStore.getState();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (activeBranchId) headers.set("X-Branch-Id", activeBranchId);
  if (operatorLabel) headers.set("X-Operator-Label", operatorLabel);
  if (sessionId) headers.set("X-Session-Id", sessionId);
  const isFormData = typeof FormData !== "undefined" && init.body instanceof FormData;
  if (!headers.has("Content-Type") && init.body && !isFormData) {
    headers.set("Content-Type", "application/json");
  }
  return fetch(`/api${path}`, { ...init, headers });
}

/** Authenticated blob fetch for protected image content URLs. */
export async function apiBlob(path: string): Promise<string> {
  const res = await apiFetch(path);
  if (!res.ok) throw new Error("blob_fetch_failed");
  const blob = await res.blob();
  return URL.createObjectURL(blob);
}
