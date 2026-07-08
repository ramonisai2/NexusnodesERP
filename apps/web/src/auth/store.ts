import { create } from "zustand";
import type { SessionClaims } from "./policy";

const TOKEN_KEY = "nexus_access_token";

type AuthState = {
  token: string | null;
  claims: SessionClaims | null;
  activeBranchId: string | null;
  setSession: (token: string, claims: SessionClaims) => void;
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
  setSession: (token, claims) => {
    sessionStorage.setItem(TOKEN_KEY, token);
    set({
      token,
      claims,
      activeBranchId: claims.branch_ids[0] ?? null,
    });
  },
  setActiveBranch: (branchId) => set({ activeBranchId: branchId }),
  logout: () => {
    sessionStorage.removeItem(TOKEN_KEY);
    set({ token: null, claims: null, activeBranchId: null });
  },
  hydrate: async () => {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (!token) return;
    try {
      const claims = await fetchClaims(token);
      get().setSession(token, claims);
    } catch {
      get().logout();
    }
  },
}));

export async function loginWithDevToken(
  persona: "analyst" | "approver" | "wh_manager" | "regional" = "analyst",
): Promise<void> {
  const res = await fetch(`/api/auth/dev-token?persona=${persona}`, { method: "POST" });
  if (!res.ok) throw new Error("dev_token_failed");
  const data = await res.json();
  useAuthStore.getState().setSession(data.access_token, data.claims);
}

export async function apiFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const { token, activeBranchId } = useAuthStore.getState();
  const headers = new Headers(init.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (activeBranchId) headers.set("X-Branch-Id", activeBranchId);
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
