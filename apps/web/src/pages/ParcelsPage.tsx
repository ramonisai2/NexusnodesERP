import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useMemo, useState } from "react";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";

type ParcelHubItem = {
  kind: string;
  id: string;
  number: string;
  parcel_kind: string;
  parcel_kind_label?: string;
  status: string;
  from_branch_id?: string;
  to_branch_id?: string;
  created_at: string;
};

type WarrantyCase = {
  id: string;
  case_number: string;
  branch_id: string;
  destination_branch_id?: string;
  sku?: string;
  serial_number?: string;
  customer_ref?: string;
  problem_description: string;
  status: string;
  parcel_kind: string;
  created_at: string;
};

type ReturnCase = {
  id: string;
  case_number: string;
  from_branch_id: string;
  to_branch_id: string;
  reason: string;
  notes?: string;
  status: string;
  parcel_kind: string;
  created_at: string;
};

const PARCEL_KINDS: { code: string; key: MessageKey }[] = [
  { code: "", key: "parcelFilterAll" },
  { code: "TRANSFER", key: "parcelKindTransfer" },
  { code: "CEDI_DISTRIBUTION", key: "parcelKindCedi" },
  { code: "DEFECTIVE", key: "parcelKindDefective" },
  { code: "WARRANTY", key: "parcelKindWarranty" },
  { code: "RETURN_TO_CEDI", key: "parcelKindReturnCedi" },
  { code: "REPAIR_OUT", key: "parcelKindRepairOut" },
  { code: "REPAIR_IN", key: "parcelKindRepairIn" },
];

const RETURN_REASONS: { code: string; key: MessageKey }[] = [
  { code: "DEFECTIVE", key: "returnReasonDefective" },
  { code: "WARRANTY", key: "returnReasonWarranty" },
  { code: "CUSTOMER_RETURN", key: "returnReasonCustomer" },
  { code: "OVERSTOCK", key: "returnReasonOverstock" },
  { code: "OTHER", key: "returnReasonOther" },
];

const parcelsNode = {
  id: "nav.parcels",
  label: "Paquetería",
  path: "/inventory/parcels",
  require: {
    permissions: ["inventory.parcel.read", "inventory.slip.read", "inventory.balance.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

function kindHref(item: ParcelHubItem): string {
  switch (item.kind) {
    case "slip":
      return "/inventory/slips";
    case "transport":
      return "/inventory/transport";
    case "transfer":
      return "/inventory/transfers";
    default:
      return "/inventory/parcels";
  }
}

function kindLabel(kind: string, t: (k: MessageKey) => string): string {
  switch (kind) {
    case "slip":
      return t("parcelKindSlip");
    case "transfer":
      return t("parcelKindTransferDoc");
    case "transport":
      return t("parcelKindTransport");
    case "warranty":
      return t("parcelKindWarrantyDoc");
    case "return":
      return t("parcelKindReturnDoc");
    default:
      return kind;
  }
}

export function ParcelsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={parcelsNode}
      fallback={
        <section className="panel">
          <h1>{t("parcelTitle")}</h1>
          <p className="error">{t("parcelForbidden")}</p>
        </section>
      }
    >
      <ParcelsPanel />
    </PolicyGuard>
  );
}

function ParcelsPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branch = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();

  const canWarranty =
    hasPermission(claims, "inventory.warranty.create") ||
    hasPermission(claims, "inventory.slip.create") ||
    hasPermission(claims, "inventory.movement.create");
  const canReturn =
    hasPermission(claims, "inventory.return.create") ||
    hasPermission(claims, "inventory.slip.create") ||
    hasPermission(claims, "inventory.movement.create");

  const otherBranches = useMemo(
    () => (claims?.branch_ids ?? []).filter((b) => b !== branch),
    [claims?.branch_ids, branch],
  );

  const [parcelKind, setParcelKind] = useState("");
  const [tab, setTab] = useState<"hub" | "warranty" | "return">("hub");

  const [wSku, setWSku] = useState("");
  const [wSerial, setWSerial] = useState("");
  const [wCustomer, setWCustomer] = useState("");
  const [wProblem, setWProblem] = useState("");
  const [wDest, setWDest] = useState("");

  const [rTo, setRTo] = useState("");
  const [rReason, setRReason] = useState("DEFECTIVE");
  const [rNotes, setRNotes] = useState("");

  const hub = useQuery({
    queryKey: ["parcels", branch, parcelKind],
    enabled: Boolean(branch),
    queryFn: async () => {
      const q = new URLSearchParams({ branch_id: branch });
      if (parcelKind) q.set("parcel_kind", parcelKind);
      const res = await apiFetch(`/inventory/parcels?${q}`);
      if (!res.ok) throw new Error("parcels_failed");
      return (await res.json()) as ParcelHubItem[];
    },
  });

  const warranties = useQuery({
    queryKey: ["warranty-cases", branch],
    enabled: Boolean(branch) && tab === "warranty",
    queryFn: async () => {
      const res = await apiFetch(`/inventory/warranty-cases?branch_id=${encodeURIComponent(branch)}`);
      if (!res.ok) throw new Error("warranty_failed");
      return (await res.json()) as WarrantyCase[];
    },
  });

  const returns = useQuery({
    queryKey: ["return-cases", branch],
    enabled: Boolean(branch) && tab === "return",
    queryFn: async () => {
      const res = await apiFetch(`/inventory/return-cases?branch_id=${encodeURIComponent(branch)}`);
      if (!res.ok) throw new Error("return_failed");
      return (await res.json()) as ReturnCase[];
    },
  });

  const createWarranty = useMutation({
    mutationFn: async () => {
      const body = {
        branch_id: branch,
        destination_branch_id: wDest || undefined,
        sku: wSku.trim() || undefined,
        serial_number: wSerial.trim() || undefined,
        customer_ref: wCustomer.trim() || undefined,
        problem_description: wProblem.trim(),
      };
      const res = await apiFetch("/inventory/warranty-cases", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error((await res.text()) || "create_failed");
      return (await res.json()) as WarrantyCase;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["warranty-cases"] });
      void qc.invalidateQueries({ queryKey: ["parcels"] });
      setWSku("");
      setWSerial("");
      setWCustomer("");
      setWProblem("");
    },
  });

  const createReturn = useMutation({
    mutationFn: async () => {
      const body = {
        from_branch_id: branch,
        to_branch_id: rTo || otherBranches[0],
        reason: rReason,
        notes: rNotes.trim(),
      };
      const res = await apiFetch("/inventory/return-cases", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error((await res.text()) || "create_failed");
      return (await res.json()) as ReturnCase;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["return-cases"] });
      void qc.invalidateQueries({ queryKey: ["parcels"] });
      setRNotes("");
    },
  });

  const selectedReturnTo = rTo || otherBranches[0] || "";

  return (
    <section className="panel parcels-page">
      <h1>{t("parcelTitle")}</h1>
      <p className="muted">{t("parcelSubtitle")}</p>
      <p className="muted tip">{t("parcelTip")}</p>

      <div className="filter-bar">
        <button type="button" className={`btn ${tab === "hub" ? "" : "secondary"}`} onClick={() => setTab("hub")}>
          {t("parcelTabHub")}
        </button>
        <button
          type="button"
          className={`btn ${tab === "warranty" ? "" : "secondary"}`}
          onClick={() => setTab("warranty")}
        >
          {t("parcelTabWarranty")}
        </button>
        <button
          type="button"
          className={`btn ${tab === "return" ? "" : "secondary"}`}
          onClick={() => setTab("return")}
        >
          {t("parcelTabReturn")}
        </button>
      </div>

      {tab === "hub" ? (
        <>
          <div className="filter-bar">
            <label className="filter-field">
              <span>{t("parcelFilterKind")}</span>
              <select value={parcelKind} onChange={(e) => setParcelKind(e.target.value)}>
                {PARCEL_KINDS.map((k) => (
                  <option key={k.code || "all"} value={k.code}>
                    {t(k.key)}
                  </option>
                ))}
              </select>
            </label>
            <p className="muted">
              {t("parcelQuickLinks")}:{" "}
              <Link to="/inventory/slips">{t("navSlips")}</Link> ·{" "}
              <Link to="/inventory/transport">{t("navTransport")}</Link> ·{" "}
              <Link to="/inventory/transfers">{t("navTransfers")}</Link>
            </p>
          </div>

          {hub.isLoading ? <p className="muted">{t("parcelLoading")}</p> : null}
          {hub.isError ? <p className="error">{t("parcelListError")}</p> : null}
          {(hub.data ?? []).length === 0 && !hub.isLoading ? <p className="muted">{t("parcelEmpty")}</p> : null}
          {(hub.data ?? []).length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("parcelColType")}</th>
                  <th>{t("parcelColNumber")}</th>
                  <th>{t("parcelColKind")}</th>
                  <th>{t("parcelColRoute")}</th>
                  <th>{t("parcelColStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {(hub.data ?? []).map((item) => (
                  <tr key={`${item.kind}-${item.id}`}>
                    <td>
                      <Link to={kindHref(item)}>{kindLabel(item.kind, t)}</Link>
                    </td>
                    <td>{item.number}</td>
                    <td>{item.parcel_kind_label || item.parcel_kind}</td>
                    <td>
                      {item.from_branch_id ? labelBranch(item.from_branch_id, locale) : "—"}
                      {" → "}
                      {item.to_branch_id ? labelBranch(item.to_branch_id, locale) : "—"}
                    </td>
                    <td>{item.status}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </>
      ) : null}

      {tab === "warranty" ? (
        <>
          {canWarranty ? (
            <form
              className="slip-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (!wProblem.trim()) return;
                createWarranty.mutate();
              }}
            >
              <h2>{t("warrantyCreate")}</h2>
              <div className="filter-bar">
                <label className="filter-field">
                  <span>{t("slipSku")}</span>
                  <input value={wSku} onChange={(e) => setWSku(e.target.value)} />
                </label>
                <label className="filter-field">
                  <span>{t("warrantySerial")}</span>
                  <input value={wSerial} onChange={(e) => setWSerial(e.target.value)} />
                </label>
                <label className="filter-field">
                  <span>{t("warrantyCustomer")}</span>
                  <input value={wCustomer} onChange={(e) => setWCustomer(e.target.value)} />
                </label>
                <label className="filter-field">
                  <span>{t("warrantyDest")}</span>
                  <select value={wDest} onChange={(e) => setWDest(e.target.value)}>
                    <option value="">{t("warrantyDestCedi")}</option>
                    {otherBranches.map((b) => (
                      <option key={b} value={b}>
                        {labelBranch(b, locale)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <label className="filter-field block">
                <span>{t("warrantyProblem")}</span>
                <textarea value={wProblem} onChange={(e) => setWProblem(e.target.value)} rows={3} required />
              </label>
              <button type="submit" className="btn" disabled={createWarranty.isPending}>
                {createWarranty.isPending ? t("warrantyCreating") : t("warrantyCreate")}
              </button>
              {createWarranty.isError ? (
                <p className="error">{friendlyApiError((createWarranty.error as Error).message, locale)}</p>
              ) : null}
              {createWarranty.isSuccess ? <p className="muted tip">{t("warrantyCreateOk")}</p> : null}
            </form>
          ) : null}

          <h2>{t("warrantyHistory")}</h2>
          {warranties.isLoading ? <p className="muted">{t("parcelLoading")}</p> : null}
          {(warranties.data ?? []).length === 0 && !warranties.isLoading ? (
            <p className="muted">{t("warrantyEmpty")}</p>
          ) : null}
          {(warranties.data ?? []).length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("parcelColNumber")}</th>
                  <th>{t("slipSku")}</th>
                  <th>{t("warrantyProblem")}</th>
                  <th>{t("parcelColStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {(warranties.data ?? []).map((c) => (
                  <tr key={c.id}>
                    <td>{c.case_number}</td>
                    <td>{c.sku || "—"}</td>
                    <td>{c.problem_description}</td>
                    <td>{c.status}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </>
      ) : null}

      {tab === "return" ? (
        <>
          {canReturn ? (
            <form
              className="slip-form"
              onSubmit={(e) => {
                e.preventDefault();
                if (!selectedReturnTo) return;
                createReturn.mutate();
              }}
            >
              <h2>{t("returnCreate")}</h2>
              <div className="filter-bar">
                <label className="filter-field">
                  <span>{t("slipTo")}</span>
                  <select value={selectedReturnTo} onChange={(e) => setRTo(e.target.value)} required>
                    {otherBranches.length === 0 ? <option value="">{t("slipNoOtherBranch")}</option> : null}
                    {otherBranches.map((b) => (
                      <option key={b} value={b}>
                        {labelBranch(b, locale)}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="filter-field">
                  <span>{t("returnReason")}</span>
                  <select value={rReason} onChange={(e) => setRReason(e.target.value)}>
                    {RETURN_REASONS.map((r) => (
                      <option key={r.code} value={r.code}>
                        {t(r.key)}
                      </option>
                    ))}
                  </select>
                </label>
              </div>
              <label className="filter-field block">
                <span>{t("slipNotes")}</span>
                <input value={rNotes} onChange={(e) => setRNotes(e.target.value)} />
              </label>
              <button type="submit" className="btn" disabled={createReturn.isPending || !selectedReturnTo}>
                {createReturn.isPending ? t("returnCreating") : t("returnCreate")}
              </button>
              {createReturn.isError ? (
                <p className="error">{friendlyApiError((createReturn.error as Error).message, locale)}</p>
              ) : null}
              {createReturn.isSuccess ? <p className="muted tip">{t("returnCreateOk")}</p> : null}
            </form>
          ) : null}

          <h2>{t("returnHistory")}</h2>
          {returns.isLoading ? <p className="muted">{t("parcelLoading")}</p> : null}
          {(returns.data ?? []).length === 0 && !returns.isLoading ? (
            <p className="muted">{t("returnEmpty")}</p>
          ) : null}
          {(returns.data ?? []).length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("parcelColNumber")}</th>
                  <th>{t("slipTo")}</th>
                  <th>{t("returnReason")}</th>
                  <th>{t("parcelColKind")}</th>
                  <th>{t("parcelColStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {(returns.data ?? []).map((c) => (
                  <tr key={c.id}>
                    <td>{c.case_number}</td>
                    <td>{labelBranch(c.to_branch_id, locale)}</td>
                    <td>{c.reason}</td>
                    <td>{c.parcel_kind}</td>
                    <td>{c.status}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </>
      ) : null}
    </section>
  );
}
