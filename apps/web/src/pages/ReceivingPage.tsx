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
};

type Receipt = {
  id: string;
  warehouse_id: string;
  supplier_name: string;
  invoice_number: string;
  status: string;
  labels?: Array<{
    sku: string;
    public_description: string;
    barcode?: string;
    effective_price?: number;
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

type DraftBox = {
  key: string;
  sku: string;
  boxes_count: string;
  units_per_box: string;
  unit_cost: string;
};

type DraftPallet = {
  key: string;
  label: string;
  boxes: DraftBox[];
};

type Shipment = {
  id: string;
  status: string;
  supplier_name: string;
  invoice_number: string;
  warehouse_id: string;
  vehicle_ref?: string;
  carrier_name?: string;
  expected_pallets: number;
  pallet_count: number;
  box_count: number;
  unit_total: number;
  pallets?: Array<{
    pallet_no: number;
    pallet_code: string;
    label: string;
    box_count: number;
    unit_total: number;
    boxes?: Array<{ sku: string; boxes_count: number; units_per_box: number; quantity: number }>;
  }>;
  sku_summary?: ReceiptLine[];
  receipt?: Receipt;
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

function newBox(): DraftBox {
  return {
    key: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    sku: "",
    boxes_count: "1",
    units_per_box: "12",
    unit_cost: "",
  };
}

function newPallet(n: number): DraftPallet {
  return {
    key: `${Date.now()}-${n}-${Math.random().toString(16).slice(2)}`,
    label: `Tarima ${n}`,
    boxes: [newBox()],
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

  const [mode, setMode] = useState<"truck" | "simple">("truck");

  const warehouses = useQuery({
    queryKey: ["warehouses", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/warehouses?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error("warehouses_failed");
      return (await res.json()) as Warehouse[];
    },
  });

  const preferredWarehouse = useMemo(() => sightBeyondSight(warehouses.data ?? []), [warehouses.data]);
  const [warehouseId, setWarehouseId] = useState("");
  const selectedWarehouse = warehouseId || preferredWarehouse?.id || "";
  const selectedMeta = (warehouses.data ?? []).find((w) => w.id === selectedWarehouse);

  const [supplier, setSupplier] = useState("");
  const [invoice, setInvoice] = useState("");
  const [invoiceDate, setInvoiceDate] = useState("");
  const [notes, setNotes] = useState("");
  const [printLabels, setPrintLabels] = useState(true);
  const [carrier, setCarrier] = useState("");
  const [vehicle, setVehicle] = useState("");
  const [driver, setDriver] = useState("");
  const [dock, setDock] = useState("");
  const [palletCount, setPalletCount] = useState(8);
  const [pallets, setPallets] = useState<DraftPallet[]>(() =>
    Array.from({ length: 8 }, (_, i) => newPallet(i + 1)),
  );
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);
  const [message, setMessage] = useState<string | null>(null);
  const [lastShipment, setLastShipment] = useState<Shipment | null>(null);
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

  const shipments = useQuery({
    queryKey: ["inbound-shipments", branchId],
    enabled: Boolean(branchId),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/inbound-shipments?branch_id=${encodeURIComponent(branchId)}&limit=12`);
      if (!res.ok) throw new Error("shipments_failed");
      return (await res.json()) as { items: Shipment[] };
    },
  });

  const resizePallets = (n: number) => {
    const count = Math.max(1, Math.min(40, n || 1));
    setPalletCount(count);
    setPallets((prev) => {
      if (prev.length === count) return prev;
      if (prev.length < count) {
        return [...prev, ...Array.from({ length: count - prev.length }, (_, i) => newPallet(prev.length + i + 1))];
      }
      return prev.slice(0, count).map((p, i) => ({ ...p, label: p.label || `Tarima ${i + 1}` }));
    });
  };

  const truckTotals = useMemo(() => {
    let boxes = 0;
    let units = 0;
    for (const p of pallets) {
      for (const b of p.boxes) {
        const bc = Number(b.boxes_count) || 0;
        const upb = Number(b.units_per_box) || 0;
        boxes += bc;
        units += bc * upb;
      }
    }
    return { boxes, units };
  }, [pallets]);

  const createAndPostSimple = useMutation({
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
    },
    onError: () => setMessage(t("recvError")),
  });

  const createAndPostTruck = useMutation({
    mutationFn: async () => {
      if (!selectedWarehouse) throw new Error("warehouse_required");
      const payloadPallets = pallets.map((p, i) => ({
        pallet_no: i + 1,
        label: p.label.trim() || `Tarima ${i + 1}`,
        boxes: p.boxes
          .map((b) => ({
            sku: b.sku.trim(),
            boxes_count: Number(b.boxes_count) || 0,
            units_per_box: Number(b.units_per_box) || 0,
            unit_cost: b.unit_cost ? Number(b.unit_cost) : undefined,
          }))
          .filter((b) => b.sku && b.boxes_count > 0 && b.units_per_box > 0),
      }));
      if (payloadPallets.some((p) => p.boxes.length === 0)) throw new Error("empty_pallet");
      const idem = `truck-${crypto.randomUUID()}`;
      const createRes = await apiFetch("/inventory/inbound-shipments", {
        method: "POST",
        headers: { "Idempotency-Key": idem, "Content-Type": "application/json" },
        body: JSON.stringify({
          branch_id: branchId,
          warehouse_id: selectedWarehouse,
          supplier_name: supplier.trim(),
          invoice_number: invoice.trim(),
          invoice_date: invoiceDate || undefined,
          carrier_name: carrier.trim() || undefined,
          vehicle_ref: vehicle.trim() || undefined,
          driver_name: driver.trim() || undefined,
          dock_door: dock.trim() || undefined,
          expected_pallets: palletCount,
          notes: notes.trim() || undefined,
          print_labels: printLabels,
          idempotency_key: idem,
          pallets: payloadPallets,
        }),
      });
      const created = await createRes.json().catch(() => ({}));
      if (!createRes.ok) throw new Error(created.detail || created.error || "create_failed");
      const postRes = await apiFetch(`/inventory/inbound-shipments/${created.id}/post`, { method: "POST" });
      const posted = await postRes.json().catch(() => ({}));
      if (!postRes.ok) throw new Error(posted.detail || posted.error || "post_failed");
      return posted as Shipment;
    },
    onSuccess: (ship) => {
      setMessage(t("recvTruckPostedOk"));
      setLastShipment(ship);
      setLastPosted(ship.receipt || null);
      setInvoice("");
      setNotes("");
      setVehicle("");
      void qc.invalidateQueries({ queryKey: ["receipts", branchId] });
      void qc.invalidateQueries({ queryKey: ["inbound-shipments", branchId] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
    },
    onError: () => setMessage(t("recvError")),
  });

  const pending = createAndPostSimple.isPending || createAndPostTruck.isPending;

  return (
    <section className="panel receiving-page">
      <h1>{t("recvTitle")}</h1>
      <p className="muted">{t("recvSubtitle")}</p>
      <p className="muted tip">{t("recvTruckCase")}</p>

      {selectedMeta?.kind === "CEDI" ? <p className="muted tip">{t("recvCediHint")}</p> : null}
      {selectedMeta?.kind === "ARRIVAL" ? <p className="muted tip">{t("recvArrivalHint")}</p> : null}

      <div className="recv-mode" role="tablist" aria-label="Modo de recepción">
        <button type="button" className={mode === "truck" ? "active" : undefined} onClick={() => setMode("truck")}>
          {t("recvModeTruck")}
        </button>
        <button type="button" className={mode === "simple" ? "active" : undefined} onClick={() => setMode("simple")}>
          {t("recvModeSimple")}
        </button>
      </div>

      <form
        className="recv-form"
        onSubmit={(e) => {
          e.preventDefault();
          setMessage(null);
          if (mode === "truck") createAndPostTruck.mutate();
          else createAndPostSimple.mutate();
        }}
      >
        <div className="recv-grid">
          <label>
            <span className="muted">{t("recvWarehouse")}</span>
            <select value={selectedWarehouse} onChange={(e) => setWarehouseId(e.target.value)} required>
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

        {mode === "truck" ? (
          <div className="recv-grid">
            <label>
              <span className="muted">{t("recvCarrier")}</span>
              <input value={carrier} onChange={(e) => setCarrier(e.target.value)} maxLength={80} />
            </label>
            <label>
              <span className="muted">{t("recvVehicle")}</span>
              <input value={vehicle} onChange={(e) => setVehicle(e.target.value)} maxLength={40} placeholder="Placas" />
            </label>
            <label>
              <span className="muted">{t("recvDriver")}</span>
              <input value={driver} onChange={(e) => setDriver(e.target.value)} maxLength={80} />
            </label>
            <label>
              <span className="muted">{t("recvDock")}</span>
              <input value={dock} onChange={(e) => setDock(e.target.value)} maxLength={20} placeholder="Andén 3" />
            </label>
            <label>
              <span className="muted">{t("recvPalletCount")}</span>
              <input
                type="number"
                min={1}
                max={40}
                value={palletCount}
                onChange={(e) => resizePallets(Number(e.target.value))}
              />
            </label>
          </div>
        ) : null}

        <label>
          <span className="muted">{t("recvNotes")}</span>
          <textarea value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} maxLength={400} />
        </label>

        <label className="recv-check">
          <input type="checkbox" checked={printLabels} onChange={(e) => setPrintLabels(e.target.checked)} />
          <span>{t("recvPrintLabels")}</span>
        </label>

        {mode === "truck" ? (
          <>
            <div className="recv-truck-summary">
              <strong>
                {palletCount} {t("recvPallets")} · {truckTotals.boxes} {t("recvBoxes")} · {truckTotals.units}{" "}
                {t("recvUnits")}
              </strong>
            </div>
            <div className="recv-pallets">
              {pallets.map((p, pi) => (
                <article key={p.key} className="recv-pallet">
                  <header>
                    <input
                      value={p.label}
                      onChange={(e) =>
                        setPallets((prev) => prev.map((x, i) => (i === pi ? { ...x, label: e.target.value } : x)))
                      }
                    />
                    <button
                      type="button"
                      className="btn secondary"
                      onClick={() =>
                        setPallets((prev) =>
                          prev.map((x, i) => (i === pi ? { ...x, boxes: [...x.boxes, newBox()] } : x)),
                        )
                      }
                    >
                      {t("recvAddBox")}
                    </button>
                  </header>
                  {p.boxes.map((b, bi) => (
                    <div className="recv-box" key={b.key}>
                      <input
                        placeholder={t("recvSku")}
                        value={b.sku}
                        onChange={(e) =>
                          setPallets((prev) =>
                            prev.map((x, i) =>
                              i === pi
                                ? {
                                    ...x,
                                    boxes: x.boxes.map((bx, j) => (j === bi ? { ...bx, sku: e.target.value } : bx)),
                                  }
                                : x,
                            ),
                          )
                        }
                        required
                      />
                      <input
                        placeholder={t("recvBoxesCount")}
                        inputMode="numeric"
                        value={b.boxes_count}
                        onChange={(e) =>
                          setPallets((prev) =>
                            prev.map((x, i) =>
                              i === pi
                                ? {
                                    ...x,
                                    boxes: x.boxes.map((bx, j) =>
                                      j === bi ? { ...bx, boxes_count: e.target.value } : bx,
                                    ),
                                  }
                                : x,
                            ),
                          )
                        }
                        required
                      />
                      <input
                        placeholder={t("recvUnitsPerBox")}
                        inputMode="decimal"
                        value={b.units_per_box}
                        onChange={(e) =>
                          setPallets((prev) =>
                            prev.map((x, i) =>
                              i === pi
                                ? {
                                    ...x,
                                    boxes: x.boxes.map((bx, j) =>
                                      j === bi ? { ...bx, units_per_box: e.target.value } : bx,
                                    ),
                                  }
                                : x,
                            ),
                          )
                        }
                        required
                      />
                      <input
                        placeholder={t("recvCost")}
                        inputMode="decimal"
                        value={b.unit_cost}
                        onChange={(e) =>
                          setPallets((prev) =>
                            prev.map((x, i) =>
                              i === pi
                                ? {
                                    ...x,
                                    boxes: x.boxes.map((bx, j) =>
                                      j === bi ? { ...bx, unit_cost: e.target.value } : bx,
                                    ),
                                  }
                                : x,
                            ),
                          )
                        }
                      />
                      <span className="muted">
                        = {(Number(b.boxes_count) || 0) * (Number(b.units_per_box) || 0)} {t("recvUnits")}
                      </span>
                    </div>
                  ))}
                </article>
              ))}
            </div>
          </>
        ) : (
          <>
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
          </>
        )}

        <div className="setup-actions">
          {mode === "simple" ? (
            <button type="button" className="btn secondary" onClick={() => setLines((prev) => [...prev, newLine()])}>
              {t("recvAddLine")}
            </button>
          ) : null}
          <button type="submit" className="btn" disabled={pending}>
            {pending ? t("recvPosting") : mode === "truck" ? t("recvPostTruck") : t("recvPost")}
          </button>
        </div>
        {message ? <p className={createAndPostSimple.isError || createAndPostTruck.isError ? "error" : "muted tip"}>{message}</p> : null}
      </form>

      {lastShipment ? (
        <div className="recv-truck-result">
          <h2>{t("recvTruckResult")}</h2>
          <p>
            <strong>
              {lastShipment.pallet_count} {t("recvPallets")}
            </strong>{" "}
            · {lastShipment.box_count} {t("recvBoxes")} · {lastShipment.unit_total} {t("recvUnits")} ·{" "}
            {lastShipment.status}
          </p>
          <ul className="plain-list">
            {(lastShipment.pallets || []).map((p) => (
              <li key={p.pallet_code}>
                <code>{p.pallet_code}</code> {p.label} — {p.box_count} {t("recvBoxes")} / {p.unit_total}{" "}
                {t("recvUnits")}
              </li>
            ))}
          </ul>
        </div>
      ) : null}

      {lastPosted?.labels && lastPosted.labels.length > 0 ? (
        <div className="recv-labels">
          <h2>{t("recvLabelsTitle")}</h2>
          <div className="label-grid">
            {lastPosted.labels.map((lbl) => (
              <article key={lbl.sku} className="label-card">
                <h3>{lbl.public_description}</h3>
                <p className="muted tip">{lbl.sku}</p>
                {lbl.barcode ? <p className="muted tip">{lbl.barcode}</p> : null}
                {lbl.effective_price != null ? <p className="label-price">${lbl.effective_price.toFixed(2)}</p> : null}
              </article>
            ))}
          </div>
        </div>
      ) : null}

      <h2>{t("recvTruckHistory")}</h2>
      <ul className="recv-history">
        {(shipments.data?.items ?? []).map((s) => (
          <li key={s.id}>
            <strong>{s.invoice_number}</strong>
            <span className="muted">
              {" "}
              · {s.supplier_name} · {s.pallet_count}/{s.expected_pallets} {t("recvPallets")} · {s.status}
              {s.vehicle_ref ? ` · ${s.vehicle_ref}` : ""}
            </span>
          </li>
        ))}
      </ul>

      <h2>{t("recvHistory")}</h2>
      {list.isLoading ? <p className="muted">{t("recvLoading")}</p> : null}
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
