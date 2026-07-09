import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type Warehouse = {
  id: string;
  branch_id: string;
  name: string;
  kind: string;
};

type Balance = {
  id: string;
  warehouse_id: string;
  branch_id: string;
  sku_id: string;
  sku: string;
  product_name?: string;
  on_hand: number;
  version: number;
};

type Movement = {
  id: string;
  warehouse_id: string;
  sku_id: string;
  movement_type: string;
  quantity: number;
  reason_code?: string;
  notes?: string;
  status: string;
  created_at: string;
  posted_by: string;
};

const REASONS_OUT = ["MERMA", "ROBO", "DAMAGE", "EXPIRED", "COUNT_VARIANCE", "OTHER"] as const;
const REASONS_IN = ["FOUND", "COUNT_VARIANCE", "OTHER"] as const;

const node = {
  id: "nav.adjustments",
  label: "Merma / Robo",
  path: "/inventory/adjustments",
  require: {
    permissions: ["inventory.adjustment.create", "inventory.movement.create"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function AdjustmentsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={node}
      fallback={
        <section className="panel">
          <h1>{t("adjTitle")}</h1>
          <p className="error">{t("adjForbidden")}</p>
        </section>
      }
    >
      <AdjustmentsPanel />
    </PolicyGuard>
  );
}

function AdjustmentsPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branchId = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();

  const [direction, setDirection] = useState<"out" | "in">("out");
  const [warehouseId, setWarehouseId] = useState("");
  const [sku, setSku] = useState("");
  const [qty, setQty] = useState("1");
  const [reason, setReason] = useState<string>("MERMA");
  const [notes, setNotes] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const [filterReason, setFilterReason] = useState("");

  const warehouses = useQuery({
    queryKey: ["warehouses", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/warehouses?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error("warehouses_failed");
      return (await res.json()) as Warehouse[];
    },
  });

  const preferredWh = useMemo(() => {
    const list = warehouses.data ?? [];
    return list.find((w) => w.kind === "STORE")?.id || list[0]?.id || "";
  }, [warehouses.data]);
  const selectedWh = warehouseId || preferredWh;

  const balances = useQuery({
    queryKey: ["balances", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch("/inventory/balances");
      if (!res.ok) throw new Error("balances_failed");
      return (await res.json()) as Balance[];
    },
  });

  const history = useQuery({
    queryKey: ["adjustments", branchId, filterReason],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const qs = new URLSearchParams({ limit: "40" });
      if (filterReason) qs.set("reason_code", filterReason);
      const res = await apiFetch(`/inventory/movements?${qs}`);
      if (!res.ok) throw new Error("movements_failed");
      const all = (await res.json()) as Movement[];
      return all.filter((m) =>
        ["ADJUST_OUT", "ADJUST_IN", "ADJUST"].includes(m.movement_type) || Boolean(m.reason_code),
      );
    },
  });

  const reasons = direction === "out" ? REASONS_OUT : REASONS_IN;

  const post = useMutation({
    mutationFn: async () => {
      const quantity = Number(qty);
      if (!selectedWh || !sku.trim() || !(quantity > 0) || !reason) throw new Error("invalid");
      const bal = (balances.data ?? []).find(
        (b) => b.sku === sku.trim() && b.warehouse_id === selectedWh,
      );
      const res = await apiFetch("/inventory/movements", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID(), "Content-Type": "application/json" },
        body: JSON.stringify({
          branch_id: branchId,
          warehouse_id: selectedWh,
          sku_id: sku.trim(),
          movement_type: direction === "out" ? "ADJUST_OUT" : "ADJUST_IN",
          quantity,
          reason_code: reason,
          notes: notes.trim() || undefined,
          expected_version: bal?.version,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.detail || body.error || (await res.text().catch(() => "failed")));
      return body as Movement;
    },
    onSuccess: () => {
      setMessage(t("adjPostedOk"));
      setNotes("");
      setQty("1");
      void qc.invalidateQueries({ queryKey: ["balances"] });
      void qc.invalidateQueries({ queryKey: ["adjustments"] });
      void qc.invalidateQueries({ queryKey: ["movements"] });
    },
    onError: (err) => setMessage(friendlyApiError((err as Error).message, locale) || t("adjError")),
  });

  return (
    <section className="panel receiving-page">
      <h1>{t("adjTitle")}</h1>
      <p className="muted">{t("adjSubtitle")}</p>
      <p className="muted tip">{t("adjTip")}</p>

      <div className="recv-mode" role="tablist">
        <button type="button" className={direction === "out" ? "active" : undefined} onClick={() => {
          setDirection("out");
          setReason("MERMA");
        }}>
          {t("adjOut")}
        </button>
        <button type="button" className={direction === "in" ? "active" : undefined} onClick={() => {
          setDirection("in");
          setReason("FOUND");
        }}>
          {t("adjIn")}
        </button>
      </div>

      <form
        className="recv-form"
        onSubmit={(e) => {
          e.preventDefault();
          setMessage(null);
          post.mutate();
        }}
      >
        <div className="recv-grid">
          <label>
            <span className="muted">{t("adjWarehouse")}</span>
            <select value={selectedWh} onChange={(e) => setWarehouseId(e.target.value)} required>
              {(warehouses.data ?? []).map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="muted">{t("adjSku")}</span>
            <input
              list="adj-sku-list"
              value={sku}
              onChange={(e) => setSku(e.target.value)}
              required
              maxLength={64}
            />
            <datalist id="adj-sku-list">
              {(balances.data ?? [])
                .filter((b) => !selectedWh || b.warehouse_id === selectedWh)
                .map((b) => (
                  <option key={b.id} value={b.sku}>
                    {b.product_name || b.sku} ({b.on_hand})
                  </option>
                ))}
            </datalist>
          </label>
          <label>
            <span className="muted">{t("adjQty")}</span>
            <input type="number" min={0.0001} step="any" value={qty} onChange={(e) => setQty(e.target.value)} required />
          </label>
          <label>
            <span className="muted">{t("adjReason")}</span>
            <select value={reason} onChange={(e) => setReason(e.target.value)} required>
              {reasons.map((r) => (
                <option key={r} value={r}>
                  {t(`adjReason_${r}` as "adjReason_MERMA")}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label>
          <span className="muted">{t("adjNotes")}</span>
          <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} maxLength={400} />
        </label>
        <button type="submit" className="btn" disabled={post.isPending}>
          {post.isPending ? t("adjPosting") : t("adjPost")}
        </button>
        {message ? <p className={post.isError ? "error" : "ok"}>{message}</p> : null}
      </form>

      <h2>{t("adjHistory")}</h2>
      <label className="recv-check" style={{ maxWidth: 280 }}>
        <span className="muted">{t("adjFilterReason")}</span>
        <select value={filterReason} onChange={(e) => setFilterReason(e.target.value)}>
          <option value="">{t("adjAllReasons")}</option>
          {[...REASONS_OUT, "FOUND"].map((r) => (
            <option key={r} value={r}>
              {t(`adjReason_${r}` as "adjReason_MERMA")}
            </option>
          ))}
        </select>
      </label>
      {history.isLoading ? <p className="muted">{t("adjLoading")}</p> : null}
      <ul className="recv-history">
        {(history.data ?? []).map((m) => (
          <li key={m.id}>
            <strong>{m.sku_id}</strong> · {m.movement_type} · {m.quantity} ·{" "}
            {m.reason_code ? t(`adjReason_${m.reason_code}` as "adjReason_MERMA") : "—"} · {m.status}
            {m.notes ? <span className="muted"> — {m.notes}</span> : null}
          </li>
        ))}
      </ul>
      {!history.isLoading && (history.data ?? []).length === 0 ? (
        <p className="muted">{t("adjNoRows")}</p>
      ) : null}
    </section>
  );
}
