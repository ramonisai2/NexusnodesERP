import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { useLocaleStore } from "../i18n/locale";

type Warehouse = {
  id: string;
  branch_id: string;
  name: string;
  kind: "STORE" | "CEDI" | "ARRIVAL" | string;
};

type ReceiptLine = {
  id?: string;
  sku: string;
  quantity: number;
  unit_cost?: number;
  label_description?: string;
  label_price?: number;
};

type Receipt = {
  id: string;
  branch_id: string;
  warehouse_id: string;
  warehouse_kind?: string;
  supplier_name: string;
  invoice_number: string;
  invoice_date?: string;
  notes?: string;
  status: string;
  print_labels: boolean;
  created_at: string;
  lines?: ReceiptLine[];
  labels?: Array<{
    sku: string;
    public_description: string;
    barcode?: string;
    effective_price?: number;
    price_mode?: string;
  }>;
};

type DraftLine = {
  key: string;
  sku: string;
  quantity: string;
  unit_cost: string;
  label_description: string;
  label_price: string;
};

const node = {
  id: "nav.receiving",
  label: "Recepción",
  path: "/inventory/receiving",
  require: {
    permissions: ["inventory.receipt.create", "inventory.movement.create", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

function newLine(): DraftLine {
  return {
    key: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    sku: "",
    quantity: "1",
    unit_cost: "",
    label_description: "",
    label_price: "",
  };
}

export function ReceivingPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={node}
      fallback={
        <section className="panel">
          <h1>{t("recvTitle")}</h1>
          <p className="error">{t("recvForbidden")}</p>
        </section>
      }
    >
      <ReceivingPanel />
    </PolicyGuard>
  );
}

function ReceivingPanel() {
  const t = useLocaleStore((s) => s.t);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branchId = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();

  const warehouses = useQuery({
    queryKey: ["warehouses", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/warehouses?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error("warehouses_failed");
      return (await res.json()) as Warehouse[];
    },
  });

  const preferredWarehouse = useMemo(() => {
    // Thundercats: Sight Beyond Sight — see the right receiving bay first.
    return sightBeyondSight(warehouses.data ?? []);
  }, [warehouses.data]);

  const [warehouseId, setWarehouseId] = useState("");
  const selectedWarehouse = warehouseId || preferredWarehouse?.id || "";
  const selectedMeta = (warehouses.data ?? []).find((w) => w.id === selectedWarehouse);

  const [supplier, setSupplier] = useState("");
  const [invoice, setInvoice] = useState("");
  const [invoiceDate, setInvoiceDate] = useState("");
  const [notes, setNotes] = useState("");
  const [printLabels, setPrintLabels] = useState(true);
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);
  const [message, setMessage] = useState<string | null>(null);
  const [lastPosted, setLastPosted] = useState<Receipt | null>(null);

  const list = useQuery({
    queryKey: ["receipts", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/receipts?branch_id=${encodeURIComponent(branchId)}&limit=20`);
      if (!res.ok) throw new Error("receipts_failed");
      return (await res.json()) as Receipt[];
    },
  });

  const createAndPost = useMutation({
    mutationFn: async () => {
      if (!selectedWarehouse) throw new Error("warehouse_required");
      const payloadLines = lines
        .map((l) => ({
          sku: l.sku.trim(),
          quantity: Number(l.quantity),
          unit_cost: l.unit_cost ? Number(l.unit_cost) : undefined,
          label_description: l.label_description.trim() || undefined,
          label_price: l.label_price ? Number(l.label_price) : undefined,
        }))
        .filter((l) => l.sku && l.quantity > 0);
      if (!payloadLines.length) throw new Error("lines_required");

      const idem = `recv-${crypto.randomUUID()}`;
      const createRes = await apiFetch("/inventory/receipts", {
        method: "POST",
        headers: { "Idempotency-Key": idem, "Content-Type": "application/json" },
        body: JSON.stringify({
          branch_id: branchId,
          warehouse_id: selectedWarehouse,
          supplier_name: supplier.trim(),
          invoice_number: invoice.trim(),
          invoice_date: invoiceDate || undefined,
          notes: notes.trim() || undefined,
          print_labels: printLabels,
          idempotency_key: idem,
          lines: payloadLines,
        }),
      });
      const created = await createRes.json().catch(() => ({}));
      if (!createRes.ok) throw new Error(created.detail || created.error || "create_failed");

      const postRes = await apiFetch(`/inventory/receipts/${created.id}/post`, { method: "POST" });
      const posted = await postRes.json().catch(() => ({}));
      if (!postRes.ok) throw new Error(posted.detail || posted.error || "post_failed");
      return posted as Receipt;
    },
    onSuccess: (rec) => {
      setMessage(t("recvPostedOk"));
      setLastPosted(rec);
      setLines([newLine()]);
      setInvoice("");
      setNotes("");
      void qc.invalidateQueries({ queryKey: ["receipts", branchId] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
      void qc.invalidateQueries({ queryKey: ["labels"] });
    },
    onError: () => setMessage(t("recvError")),
  });

  return (
    <section className="panel receiving-page">
      <h1>{t("recvTitle")}</h1>
      <p className="muted">{t("recvSubtitle")}</p>

      {selectedMeta?.kind === "CEDI" ? <p className="muted tip">{t("recvCediHint")}</p> : null}
      {selectedMeta?.kind === "ARRIVAL" ? <p className="muted tip">{t("recvArrivalHint")}</p> : null}

      <form
        className="recv-form"
        onSubmit={(e) => {
          e.preventDefault();
          setMessage(null);
          createAndPost.mutate();
        }}
      >
        <div className="recv-grid">
          <label>
            <span className="muted">{t("recvWarehouse")}</span>
            <select
              value={selectedWarehouse}
              onChange={(e) => setWarehouseId(e.target.value)}
              required
            >
              {(warehouses.data ?? []).map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name} ({kindLabel(w.kind, t)})
                </option>
              ))}
            </select>
          </label>
          <label>
            <span className="muted">{t("recvSupplier")}</span>
            <input value={supplier} onChange={(e) => setSupplier(e.target.value)} required maxLength={120} />
          </label>
          <label>
            <span className="muted">{t("recvInvoice")}</span>
            <input value={invoice} onChange={(e) => setInvoice(e.target.value)} required maxLength={80} />
          </label>
          <label>
            <span className="muted">{t("recvInvoiceDate")}</span>
            <input type="date" value={invoiceDate} onChange={(e) => setInvoiceDate(e.target.value)} />
          </label>
        </div>

        <label>
          <span className="muted">{t("recvNotes")}</span>
          <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} maxLength={400} />
        </label>

        <label className="recv-check">
          <input type="checkbox" checked={printLabels} onChange={(e) => setPrintLabels(e.target.checked)} />
          <span>{t("recvPrintLabels")}</span>
        </label>

        <h2>{t("recvLines")}</h2>
        <div className="recv-lines">
          {lines.map((line, idx) => (
            <div className="recv-line" key={line.key}>
              <input
                placeholder={t("recvSku")}
                value={line.sku}
                onChange={(e) => updateLine(idx, { sku: e.target.value })}
                required
              />
              <input
                placeholder={t("recvQty")}
                inputMode="decimal"
                value={line.quantity}
                onChange={(e) => updateLine(idx, { quantity: e.target.value })}
                required
              />
              <input
                placeholder={t("recvCost")}
                inputMode="decimal"
                value={line.unit_cost}
                onChange={(e) => updateLine(idx, { unit_cost: e.target.value })}
              />
              <input
                placeholder={t("recvLabelDesc")}
                value={line.label_description}
                onChange={(e) => updateLine(idx, { label_description: e.target.value })}
              />
              <input
                placeholder={t("recvLabelPrice")}
                inputMode="decimal"
                value={line.label_price}
                onChange={(e) => updateLine(idx, { label_price: e.target.value })}
              />
            </div>
          ))}
        </div>
        <div className="setup-actions">
          <button type="button" className="btn secondary" onClick={() => setLines((prev) => [...prev, newLine()])}>
            {t("recvAddLine")}
          </button>
          <button type="submit" className="btn" disabled={createAndPost.isPending}>
            {createAndPost.isPending ? t("recvPosting") : t("recvPost")}
          </button>
        </div>
        {message ? <p className={createAndPost.isError ? "error" : "muted tip"}>{message}</p> : null}
      </form>

      {lastPosted?.labels && lastPosted.labels.length > 0 ? (
        <div className="recv-labels">
          <h2>{t("recvLabelsTitle")}</h2>
          <div className="label-grid">
            {lastPosted.labels.map((lbl) => (
              <article key={lbl.sku} className="label-card">
                <h3>{lbl.public_description}</h3>
                <p className="muted tip">{lbl.sku}</p>
                {lbl.barcode ? <p className="muted tip">{lbl.barcode}</p> : null}
                {lbl.effective_price != null ? (
                  <p className="label-price">${lbl.effective_price.toFixed(2)}</p>
                ) : null}
              </article>
            ))}
          </div>
        </div>
      ) : null}

      <h2>{t("recvHistory")}</h2>
      {list.isLoading ? <p className="muted">{t("recvLoading")}</p> : null}
      {list.isError ? <p className="error">{t("recvListError")}</p> : null}
      <ul className="recv-history">
        {(list.data ?? []).map((r) => (
          <li key={r.id}>
            <strong>{r.invoice_number || r.id.slice(0, 8)}</strong>
            <span className="muted">
              {" "}
              · {r.supplier_name} · {r.warehouse_id} · {r.status}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );

  function updateLine(idx: number, patch: Partial<DraftLine>) {
    setLines((prev) => prev.map((l, i) => (i === idx ? { ...l, ...patch } : l)));
  }
}

function kindLabel(kind: string, t: (k: "recvKindStore" | "recvKindCedi" | "recvKindArrival") => string) {
  if (kind === "CEDI") return t("recvKindCedi");
  if (kind === "ARRIVAL") return t("recvKindArrival");
  return t("recvKindStore");
}

/** Thundercats — Sight Beyond Sight: prefer ARRIVAL, then CEDI, then any warehouse. */
export function sightBeyondSight(list: Warehouse[]): Warehouse | undefined {
  return list.find((w) => w.kind === "ARRIVAL") || list.find((w) => w.kind === "CEDI") || list[0];
}
