import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";

type Warehouse = { id: string; branch_id: string; name: string; kind: string };
type TransferLine = {
  id?: string;
  sku: string;
  quantity: number;
  out_movement_id?: string;
  in_movement_id?: string;
};
type InventoryTransfer = {
  id: string;
  transfer_number: string;
  from_branch_id: string;
  to_branch_id: string;
  from_warehouse_id: string;
  to_warehouse_id: string;
  status: string;
  notes?: string;
  created_at: string;
  shipped_at?: string;
  received_at?: string;
  lines?: TransferLine[];
};

type DraftLine = { key: string; sku: string; quantity: string };

const transfersNode = {
  id: "nav.transfers",
  label: "Traslados",
  path: "/inventory/transfers",
  require: {
    permissions: ["inventory.transfer.read", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

function newLine(): DraftLine {
  return {
    key: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    sku: "",
    quantity: "1",
  };
}

export function TransfersPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={transfersNode}
      fallback={
        <section className="panel">
          <h1>{t("transferTitle")}</h1>
          <p className="error">{t("transferForbidden")}</p>
        </section>
      }
    >
      <TransfersPanel />
    </PolicyGuard>
  );
}

function TransfersPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const fromBranch = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();
  const canCreate =
    hasPermission(claims, "inventory.transfer.create") ||
    hasPermission(claims, "inventory.movement.create");
  const canShip =
    hasPermission(claims, "inventory.transfer.ship") ||
    hasPermission(claims, "inventory.transfer.create") ||
    hasPermission(claims, "inventory.movement.create");
  const canReceive =
    hasPermission(claims, "inventory.transfer.receive") ||
    hasPermission(claims, "inventory.transfer.create") ||
    hasPermission(claims, "inventory.movement.create");
  const canCancel =
    hasPermission(claims, "inventory.transfer.cancel") ||
    claims?.roles?.some((r) =>
      ["warehouse_manager", "regional_manager", "store_owner", "platform_admin"].includes(r),
    );

  const [direction, setDirection] = useState<"from" | "to">("from");
  const [toBranch, setToBranch] = useState("");
  const [fromWh, setFromWh] = useState("");
  const [toWh, setToWh] = useState("");
  const [notes, setNotes] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);
  const [error, setError] = useState("");
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const otherBranches = useMemo(
    () => (claims?.branch_ids ?? []).filter((b) => b !== fromBranch),
    [claims?.branch_ids, fromBranch],
  );

  const warehouses = useQuery({
    queryKey: ["warehouses", fromBranch, toBranch],
    queryFn: async () => {
      const res = await apiFetch(`/inventory/warehouses`);
      if (!res.ok) throw new Error("warehouses_failed");
      return (await res.json()) as Warehouse[];
    },
  });

  const fromWarehouses = useMemo(
    () => (warehouses.data ?? []).filter((w) => w.branch_id === fromBranch),
    [warehouses.data, fromBranch],
  );
  const toWarehouses = useMemo(
    () => (warehouses.data ?? []).filter((w) => w.branch_id === (toBranch || otherBranches[0])),
    [warehouses.data, toBranch, otherBranches],
  );

  const list = useQuery({
    queryKey: ["transfers", fromBranch, direction],
    enabled: !!fromBranch,
    queryFn: async () => {
      const params = new URLSearchParams({ branch_id: fromBranch, direction });
      const res = await apiFetch(`/inventory/transfers?${params}`);
      if (!res.ok) throw new Error("list_failed");
      return (await res.json()) as InventoryTransfer[];
    },
  });

  const detail = useQuery({
    queryKey: ["transfer", selectedId],
    enabled: !!selectedId,
    queryFn: async () => {
      const res = await apiFetch(`/inventory/transfers/${selectedId}`);
      if (!res.ok) throw new Error("detail_failed");
      return (await res.json()) as InventoryTransfer;
    },
  });

  const create = useMutation({
    mutationFn: async () => {
      const destBranch = toBranch || otherBranches[0];
      const payload = {
        from_branch_id: fromBranch,
        to_branch_id: destBranch,
        from_warehouse_id: fromWh || fromWarehouses[0]?.id,
        to_warehouse_id: toWh || toWarehouses[0]?.id,
        notes,
        idempotency_key: `trf-${Date.now()}-${Math.random().toString(16).slice(2)}`,
        lines: lines
          .filter((l) => l.sku.trim())
          .map((l) => ({ sku: l.sku.trim(), quantity: Number(l.quantity) || 0 })),
      };
      const res = await apiFetch("/inventory/transfers", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Idempotency-Key": payload.idempotency_key },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as InventoryTransfer;
    },
    onSuccess: (tr) => {
      setError("");
      setNotes("");
      setLines([newLine()]);
      setSelectedId(tr.id);
      void qc.invalidateQueries({ queryKey: ["transfers"] });
    },
    onError: (e: Error) => setError(friendlyApiError(e.message, locale) || t("transferCreateError")),
  });

  const ship = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transfers/${id}/ship`, { method: "POST" });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["transfers"] });
      void qc.invalidateQueries({ queryKey: ["transfer"] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
    },
  });

  const receive = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transfers/${id}/receive`, { method: "POST" });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["transfers"] });
      void qc.invalidateQueries({ queryKey: ["transfer"] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
    },
  });

  const cancel = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transfers/${id}/cancel`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: "cancelled_from_ui" }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["transfers"] });
      void qc.invalidateQueries({ queryKey: ["transfer"] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
    },
  });

  const selected = detail.data;

  return (
    <section className="panel transfer-page">
      <header className="transfer-header">
        <div>
          <h1>{t("transferTitle")}</h1>
          <p className="muted">{t("transferSubtitle")}</p>
        </div>
        <div className="transfer-dir" role="group" aria-label={t("transferDirection")}>
          <button
            type="button"
            className={`btn${direction === "from" ? " primary" : ""}`}
            onClick={() => setDirection("from")}
          >
            {t("transferOutgoing")}
          </button>
          <button
            type="button"
            className={`btn${direction === "to" ? " primary" : ""}`}
            onClick={() => setDirection("to")}
          >
            {t("transferIncoming")}
          </button>
        </div>
      </header>

      <div className="transfer-layout">
        <div className="transfer-list">
          {list.isLoading ? <p className="muted">{t("transferLoading")}</p> : null}
          {(list.data?.length ?? 0) === 0 && !list.isLoading ? (
            <p className="muted">{t("transferEmpty")}</p>
          ) : null}
          {list.data?.map((tr) => (
            <button
              key={tr.id}
              type="button"
              className={`transfer-row${selectedId === tr.id ? " selected" : ""}`}
              onClick={() => setSelectedId(tr.id)}
            >
              <strong>{tr.transfer_number}</strong>
              <span className={`transfer-status status-${tr.status.toLowerCase()}`}>{statusLabel(tr.status, t)}</span>
              <span className="muted">
                {labelBranch(tr.from_branch_id, locale)} → {labelBranch(tr.to_branch_id, locale)}
              </span>
            </button>
          ))}
        </div>

        <div className="transfer-main">
          {selected ? (
            <article className="transfer-detail">
              <h2>{selected.transfer_number}</h2>
              <p className="muted">
                {labelBranch(selected.from_branch_id, locale)} / {selected.from_warehouse_id}
                {" → "}
                {labelBranch(selected.to_branch_id, locale)} / {selected.to_warehouse_id}
              </p>
              <p>
                <span className={`transfer-status status-${selected.status.toLowerCase()}`}>
                  {statusLabel(selected.status, t)}
                </span>
              </p>
              <ul className="transfer-lines">
                {selected.lines?.map((l) => (
                  <li key={l.id || l.sku}>
                    <strong>{l.sku}</strong> × {l.quantity}
                  </li>
                ))}
              </ul>
              <div className="transfer-actions">
                {selected.status === "DRAFT" && canShip ? (
                  <button
                    type="button"
                    className="btn primary"
                    disabled={ship.isPending}
                    onClick={() => ship.mutate(selected.id)}
                  >
                    {t("transferShip")}
                  </button>
                ) : null}
                {selected.status === "IN_TRANSIT" && canReceive ? (
                  <button
                    type="button"
                    className="btn primary"
                    disabled={receive.isPending}
                    onClick={() => receive.mutate(selected.id)}
                  >
                    {t("transferReceive")}
                  </button>
                ) : null}
                {(selected.status === "DRAFT" || selected.status === "IN_TRANSIT") && canCancel ? (
                  <button
                    type="button"
                    className="btn"
                    disabled={cancel.isPending}
                    onClick={() => cancel.mutate(selected.id)}
                  >
                    {t("transferCancel")}
                  </button>
                ) : null}
              </div>
              {ship.isError || receive.isError || cancel.isError ? (
                <p className="error">{t("transferActionError")}</p>
              ) : null}
            </article>
          ) : (
            <p className="muted">{t("transferSelect")}</p>
          )}

          {canCreate && direction === "from" ? (
            <form
              className="transfer-compose"
              onSubmit={(e) => {
                e.preventDefault();
                if (!lines.some((l) => l.sku.trim())) {
                  setError(t("transferLinesRequired"));
                  return;
                }
                create.mutate();
              }}
            >
              <h2>{t("transferCreate")}</h2>
              <label className="mail-field">
                <span>{t("transferToBranch")}</span>
                <select
                  value={toBranch || otherBranches[0] || ""}
                  onChange={(e) => {
                    setToBranch(e.target.value);
                    setToWh("");
                  }}
                >
                  {otherBranches.map((b) => (
                    <option key={b} value={b}>
                      {labelBranch(b, locale)}
                    </option>
                  ))}
                </select>
              </label>
              <div className="transfer-wh-row">
                <label className="mail-field">
                  <span>{t("transferFromWh")}</span>
                  <select value={fromWh || fromWarehouses[0]?.id || ""} onChange={(e) => setFromWh(e.target.value)}>
                    {fromWarehouses.map((w) => (
                      <option key={w.id} value={w.id}>
                        {w.name} ({w.id})
                      </option>
                    ))}
                  </select>
                </label>
                <label className="mail-field">
                  <span>{t("transferToWh")}</span>
                  <select value={toWh || toWarehouses[0]?.id || ""} onChange={(e) => setToWh(e.target.value)}>
                    {toWarehouses.map((w) => (
                      <option key={w.id} value={w.id}>
                        {w.name} ({w.id})
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <label className="mail-field">
                <span>{t("transferNotes")}</span>
                <textarea rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} />
              </label>
              <div className="transfer-draft-lines">
                {lines.map((line, idx) => (
                  <div key={line.key} className="transfer-draft-line">
                    <input
                      placeholder={t("transferSku")}
                      value={line.sku}
                      onChange={(e) =>
                        setLines((prev) => prev.map((l, i) => (i === idx ? { ...l, sku: e.target.value } : l)))
                      }
                    />
                    <input
                      type="number"
                      min="0.01"
                      step="any"
                      placeholder={t("transferQty")}
                      value={line.quantity}
                      onChange={(e) =>
                        setLines((prev) =>
                          prev.map((l, i) => (i === idx ? { ...l, quantity: e.target.value } : l)),
                        )
                      }
                    />
                  </div>
                ))}
                <button type="button" className="btn" onClick={() => setLines((p) => [...p, newLine()])}>
                  {t("transferAddLine")}
                </button>
              </div>
              {error ? <p className="error">{error}</p> : null}
              <button type="submit" className="btn primary" disabled={create.isPending}>
                {create.isPending ? t("transferCreating") : t("transferCreate")}
              </button>
            </form>
          ) : null}
        </div>
      </div>
    </section>
  );
}

function statusLabel(status: string, t: (k: MessageKey) => string) {
  switch (status) {
    case "DRAFT":
      return t("transferStatusDraft");
    case "IN_TRANSIT":
      return t("transferStatusTransit");
    case "RECEIVED":
      return t("transferStatusReceived");
    case "CANCELLED":
      return t("transferStatusCancelled");
    default:
      return status;
  }
}
