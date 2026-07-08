import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";

type SlipLine = {
  id?: string;
  sku?: string;
  description: string;
  quantity: number;
};

type ShippingSlip = {
  id: string;
  slip_number: string;
  from_branch_id: string;
  to_branch_id: string;
  container_type: string;
  container_label?: string;
  description: string;
  contents_summary?: string;
  quantity_units: number;
  status: string;
  operator_label?: string;
  notes?: string;
  created_at: string;
  printed_at?: string;
  lines?: SlipLine[];
};

type DraftLine = { key: string; sku: string; description: string; quantity: string };

const CONTAINERS = [
  { code: "ENVELOPE", key: "slipContainerEnvelope" as const },
  { code: "BOX", key: "slipContainerBox" as const },
  { code: "PLASTIC_BOX", key: "slipContainerPlastic" as const },
  { code: "BUNDLE", key: "slipContainerBundle" as const },
  { code: "ORIGINAL_PACK", key: "slipContainerOriginal" as const },
];

const slipsNode = NAV_NODES.find((n) => n.id === "nav.slips")!;

function newLine(): DraftLine {
  return {
    key: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    sku: "",
    description: "",
    quantity: "1",
  };
}

export function ShippingSlipsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={slipsNode}
      fallback={
        <section className="panel">
          <h1>{t("slipTitle")}</h1>
          <p className="error">{t("slipForbidden")}</p>
        </section>
      }
    >
      <ShippingSlipsPanel />
    </PolicyGuard>
  );
}

function ShippingSlipsPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const fromBranch = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();
  const canCreate = hasPermission(claims, "inventory.slip.create") || hasPermission(claims, "inventory.movement.create");

  const otherBranches = useMemo(
    () => (claims?.branch_ids ?? []).filter((b) => b !== fromBranch),
    [claims?.branch_ids, fromBranch],
  );

  const [toBranch, setToBranch] = useState("");
  const [container, setContainer] = useState("BOX");
  const [description, setDescription] = useState("");
  const [contents, setContents] = useState("");
  const [qtyUnits, setQtyUnits] = useState("1");
  const [notes, setNotes] = useState("");
  const [lines, setLines] = useState<DraftLine[]>([newLine()]);
  const [printSlip, setPrintSlip] = useState<ShippingSlip | null>(null);

  const selectedTo = toBranch || otherBranches[0] || "";

  const list = useQuery({
    queryKey: ["slips", fromBranch],
    enabled: Boolean(fromBranch),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/slips?branch_id=${encodeURIComponent(fromBranch)}&direction=from`);
      if (!res.ok) throw new Error("slips_failed");
      return (await res.json()) as ShippingSlip[];
    },
  });

  const create = useMutation({
    mutationFn: async () => {
      const body = {
        from_branch_id: fromBranch,
        to_branch_id: selectedTo,
        container_type: container,
        description: description.trim(),
        contents_summary: contents.trim(),
        quantity_units: Math.max(1, Number(qtyUnits) || 1),
        notes: notes.trim(),
        lines: lines
          .filter((l) => l.sku.trim() || l.description.trim())
          .map((l) => ({
            sku: l.sku.trim(),
            description: l.description.trim() || l.sku.trim(),
            quantity: Math.max(0.001, Number(l.quantity) || 1),
          })),
      };
      const res = await apiFetch("/inventory/slips", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(body),
      });
      if (!res.ok) {
        throw new Error((await res.text()) || "create_failed");
      }
      return (await res.json()) as ShippingSlip;
    },
    onSuccess: (slip) => {
      void qc.invalidateQueries({ queryKey: ["slips"] });
      setPrintSlip(slip);
      setDescription("");
      setContents("");
      setNotes("");
      setLines([newLine()]);
    },
  });

  const markPrinted = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/slips/${id}/print`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()) || "print_failed");
      return (await res.json()) as ShippingSlip;
    },
    onSuccess: (slip) => {
      void qc.invalidateQueries({ queryKey: ["slips"] });
      setPrintSlip(slip);
    },
  });

  function openPrint(slip: ShippingSlip) {
    setPrintSlip(slip);
    void markPrinted.mutateAsync(slip.id).then(() => {
      window.setTimeout(() => window.print(), 200);
    });
  }

  return (
    <section className="panel slips-page">
      <h1>{t("slipTitle")}</h1>
      <p className="muted">{t("slipSubtitle")}</p>
      <p className="muted tip">{t("slipTip")}</p>

      {canCreate ? (
        <form
          className="slip-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!selectedTo || !description.trim()) return;
            create.mutate();
          }}
        >
          <div className="filter-bar">
            <label className="filter-field">
              <span>{t("slipFrom")}</span>
              <strong>{labelBranch(fromBranch, locale)}</strong>
            </label>
            <label className="filter-field">
              <span>{t("slipTo")}</span>
              <select value={selectedTo} onChange={(e) => setToBranch(e.target.value)} required>
                {otherBranches.length === 0 ? <option value="">{t("slipNoOtherBranch")}</option> : null}
                {otherBranches.map((b) => (
                  <option key={b} value={b}>
                    {labelBranch(b, locale)}
                  </option>
                ))}
              </select>
            </label>
            <label className="filter-field">
              <span>{t("slipContainer")}</span>
              <select value={container} onChange={(e) => setContainer(e.target.value)}>
                {CONTAINERS.map((c) => (
                  <option key={c.code} value={c.code}>
                    {t(c.key)}
                  </option>
                ))}
              </select>
            </label>
            <label className="filter-field">
              <span>{t("slipQtyUnits")}</span>
              <input type="number" min={1} value={qtyUnits} onChange={(e) => setQtyUnits(e.target.value)} />
            </label>
          </div>

          <label className="filter-field block">
            <span>{t("slipDescription")}</span>
            <textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={2}
              placeholder={t("slipDescriptionPh")}
              required
            />
          </label>
          <label className="filter-field block">
            <span>{t("slipContents")}</span>
            <input value={contents} onChange={(e) => setContents(e.target.value)} placeholder={t("slipContentsPh")} />
          </label>
          <label className="filter-field block">
            <span>{t("slipNotes")}</span>
            <input value={notes} onChange={(e) => setNotes(e.target.value)} />
          </label>

          <h2>{t("slipLines")}</h2>
          {lines.map((line, idx) => (
            <div className="filter-bar" key={line.key}>
              <label className="filter-field">
                <span>{t("slipSku")}</span>
                <input
                  value={line.sku}
                  onChange={(e) => {
                    const next = [...lines];
                    next[idx] = { ...line, sku: e.target.value };
                    setLines(next);
                  }}
                />
              </label>
              <label className="filter-field grow">
                <span>{t("slipLineDesc")}</span>
                <input
                  value={line.description}
                  onChange={(e) => {
                    const next = [...lines];
                    next[idx] = { ...line, description: e.target.value };
                    setLines(next);
                  }}
                />
              </label>
              <label className="filter-field">
                <span>{t("slipLineQty")}</span>
                <input
                  type="number"
                  min={0.001}
                  step="any"
                  value={line.quantity}
                  onChange={(e) => {
                    const next = [...lines];
                    next[idx] = { ...line, quantity: e.target.value };
                    setLines(next);
                  }}
                />
              </label>
            </div>
          ))}
          <div className="slip-form-actions">
            <button type="button" className="btn secondary" onClick={() => setLines((v) => [...v, newLine()])}>
              {t("slipAddLine")}
            </button>
            <button type="submit" className="btn" disabled={create.isPending || !selectedTo}>
              {create.isPending ? t("slipCreating") : t("slipCreate")}
            </button>
          </div>
          {create.isError ? (
            <p className="error">{friendlyApiError((create.error as Error).message, locale)}</p>
          ) : null}
          {create.isSuccess ? <p className="muted tip">{t("slipCreateOk")}</p> : null}
        </form>
      ) : null}

      <h2>{t("slipHistory")}</h2>
      {list.isLoading ? <p className="muted">{t("slipLoading")}</p> : null}
      {list.isError ? <p className="error">{t("slipListError")}</p> : null}
      {(list.data ?? []).length === 0 && !list.isLoading ? <p className="muted">{t("slipEmpty")}</p> : null}
      {(list.data ?? []).length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>{t("slipColNumber")}</th>
              <th>{t("slipColTo")}</th>
              <th>{t("slipContainer")}</th>
              <th>{t("slipColStatus")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(list.data ?? []).map((s) => (
              <tr key={s.id}>
                <td>{s.slip_number}</td>
                <td>{labelBranch(s.to_branch_id, locale)}</td>
                <td>{s.container_label || s.container_type}</td>
                <td>{s.status}</td>
                <td>
                  <button type="button" className="btn secondary" onClick={() => openPrint(s)}>
                    {t("slipPrint")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}

      {printSlip ? (
        <div className="slip-print-sheet" aria-label={t("slipPrintSheet")}>
          <div className="slip-print-actions no-print">
            <button type="button" className="btn" onClick={() => window.print()}>
              {t("slipPrint")}
            </button>
            <button type="button" className="btn secondary" onClick={() => setPrintSlip(null)}>
              {t("slipClosePreview")}
            </button>
          </div>
          <article className="papeleta">
            <header>
              <p className="papeleta-kicker">{t("slipPrintSheet")}</p>
              <h2>{printSlip.slip_number}</h2>
            </header>
            <dl className="papeleta-meta">
              <div>
                <dt>{t("slipFrom")}</dt>
                <dd>{labelBranch(printSlip.from_branch_id, locale)}</dd>
              </div>
              <div>
                <dt>{t("slipTo")}</dt>
                <dd>{labelBranch(printSlip.to_branch_id, locale)}</dd>
              </div>
              <div>
                <dt>{t("slipContainer")}</dt>
                <dd className="papeleta-container">{printSlip.container_label || printSlip.container_type}</dd>
              </div>
              <div>
                <dt>{t("slipQtyUnits")}</dt>
                <dd>{printSlip.quantity_units}</dd>
              </div>
            </dl>
            <p className="papeleta-desc">{printSlip.description}</p>
            {printSlip.contents_summary ? (
              <p>
                <strong>{t("slipContents")}:</strong> {printSlip.contents_summary}
              </p>
            ) : null}
            {(printSlip.lines ?? []).length > 0 ? (
              <ul className="papeleta-lines">
                {printSlip.lines!.map((l) => (
                  <li key={l.id ?? `${l.sku}-${l.description}`}>
                    {l.sku ? <code>{l.sku}</code> : null} {l.description} × {l.quantity}
                  </li>
                ))}
              </ul>
            ) : null}
            {printSlip.operator_label ? (
              <p className="muted">
                {t("slipOperator")}: {printSlip.operator_label}
              </p>
            ) : null}
            {printSlip.notes ? <p className="muted">{printSlip.notes}</p> : null}
            <footer className="papeleta-footer">{t("slipStickHint")}</footer>
          </article>
        </div>
      ) : null}
    </section>
  );
}
