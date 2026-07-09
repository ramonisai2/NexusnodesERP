import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";

type Department = {
  code: string;
  name: string;
  branch_id: string;
};

type ShippingSlip = {
  id: string;
  slip_number: string;
  container_type: string;
  container_label?: string;
  description: string;
  status: string;
};

type LinkedSlip = {
  id: string;
  slip_number: string;
  container_type: string;
  container_label?: string;
  description: string;
  status: string;
};

type TransportSection = {
  id?: string;
  department_code: string;
  department_name: string;
  notes: string;
  slips?: LinkedSlip[];
};

type TransportSheet = {
  id: string;
  sheet_number: string;
  from_branch_id: string;
  to_branch_id: string;
  carrier_name?: string;
  vehicle_ref?: string;
  driver_name?: string;
  parcel_kind?: string;
  parcel_kind_label?: string;
  status: string;
  notes?: string;
  operator_label?: string;
  created_at: string;
  printed_at?: string;
  sections?: TransportSection[];
};

const PARCEL_KINDS = [
  { code: "TRANSFER", key: "parcelKindTransfer" as const },
  { code: "CEDI_DISTRIBUTION", key: "parcelKindCedi" as const },
  { code: "DEFECTIVE", key: "parcelKindDefective" as const },
  { code: "WARRANTY", key: "parcelKindWarranty" as const },
  { code: "RETURN_TO_CEDI", key: "parcelKindReturnCedi" as const },
  { code: "REPAIR_OUT", key: "parcelKindRepairOut" as const },
  { code: "REPAIR_IN", key: "parcelKindRepairIn" as const },
];

type DraftSection = {
  key: string;
  department_code: string;
  notes: string;
  slip_ids: string[];
};

const transportNode = NAV_NODES.find((n) => n.id === "nav.transport")!;

function newSection(deptCode = ""): DraftSection {
  return {
    key: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
    department_code: deptCode,
    notes: "",
    slip_ids: [],
  };
}

export function TransportSheetsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={transportNode}
      fallback={
        <section className="panel">
          <h1>{t("trTitle")}</h1>
          <p className="error">{t("trForbidden")}</p>
        </section>
      }
    >
      <TransportSheetsPanel />
    </PolicyGuard>
  );
}

function TransportSheetsPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const fromBranch = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();
  const canCreate =
    hasPermission(claims, "inventory.transport.create") || hasPermission(claims, "inventory.movement.create");
  const canDepart =
    hasPermission(claims, "inventory.transport.depart") ||
    hasPermission(claims, "inventory.transport.create") ||
    hasPermission(claims, "inventory.movement.create");
  const canDeliver =
    hasPermission(claims, "inventory.transport.deliver") ||
    hasPermission(claims, "inventory.transport.create") ||
    hasPermission(claims, "inventory.movement.create");

  const otherBranches = useMemo(
    () => (claims?.branch_ids ?? []).filter((b) => b !== fromBranch),
    [claims?.branch_ids, fromBranch],
  );

  const [toBranch, setToBranch] = useState("");
  const [carrier, setCarrier] = useState("");
  const [vehicle, setVehicle] = useState("");
  const [driver, setDriver] = useState("");
  const [parcelKind, setParcelKind] = useState("TRANSFER");
  const [notes, setNotes] = useState("");
  const [sections, setSections] = useState<DraftSection[]>([newSection()]);
  const [printSheet, setPrintSheet] = useState<TransportSheet | null>(null);

  const selectedTo = toBranch || otherBranches[0] || "";

  const departments = useQuery({
    queryKey: ["departments", fromBranch],
    enabled: Boolean(fromBranch),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/departments?branch_id=${encodeURIComponent(fromBranch)}`);
      if (!res.ok) throw new Error("departments_failed");
      return (await res.json()) as Department[];
    },
  });

  const slips = useQuery({
    queryKey: ["slips-for-transport", fromBranch],
    enabled: Boolean(fromBranch),
    queryFn: async () => {
      const res = await apiFetch(`/inventory/slips?branch_id=${encodeURIComponent(fromBranch)}&direction=from&limit=50`);
      if (!res.ok) throw new Error("slips_failed");
      return (await res.json()) as ShippingSlip[];
    },
  });

  const list = useQuery({
    queryKey: ["transport-sheets", fromBranch],
    enabled: Boolean(fromBranch),
    queryFn: async () => {
      const res = await apiFetch(
        `/inventory/transport-sheets?branch_id=${encodeURIComponent(fromBranch)}&direction=from`,
      );
      if (!res.ok) throw new Error("transport_failed");
      return (await res.json()) as TransportSheet[];
    },
  });

  const create = useMutation({
    mutationFn: async () => {
      const deptMap = new Map((departments.data ?? []).map((d) => [d.code, d.name]));
      const body = {
        from_branch_id: fromBranch,
        to_branch_id: selectedTo,
        carrier_name: carrier.trim(),
        vehicle_ref: vehicle.trim(),
        driver_name: driver.trim(),
        parcel_kind: parcelKind,
        notes: notes.trim(),
        sections: sections
          .filter((s) => s.department_code && (s.notes.trim() || s.slip_ids.length > 0))
          .map((s) => ({
            department_code: s.department_code,
            department_name: deptMap.get(s.department_code) || s.department_code,
            notes: s.notes.trim(),
            slip_ids: s.slip_ids,
          })),
      };
      if (body.sections.length === 0) {
        throw new Error(locale === "en" ? "Add at least one department with notes or slips" : "Agrega al menos un departamento con notas o papeletas");
      }
      const res = await apiFetch("/inventory/transport-sheets", {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error((await res.text()) || "create_failed");
      return (await res.json()) as TransportSheet;
    },
    onSuccess: (sheet) => {
      void qc.invalidateQueries({ queryKey: ["transport-sheets"] });
      void qc.invalidateQueries({ queryKey: ["parcels"] });
      setPrintSheet(sheet);
      setCarrier("");
      setVehicle("");
      setDriver("");
      setNotes("");
      setSections([newSection()]);
    },
  });

  const markPrinted = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transport-sheets/${id}/print`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()) || "print_failed");
      return (await res.json()) as TransportSheet;
    },
    onSuccess: (sheet) => {
      void qc.invalidateQueries({ queryKey: ["transport-sheets"] });
      setPrintSheet(sheet);
    },
  });

  const departSheet = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transport-sheets/${id}/depart`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()) || "depart_failed");
      return (await res.json()) as TransportSheet;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["transport-sheets"] });
      void qc.invalidateQueries({ queryKey: ["slips"] });
      void qc.invalidateQueries({ queryKey: ["parcels"] });
    },
  });

  const deliverSheet = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/inventory/transport-sheets/${id}/deliver`, { method: "POST" });
      if (!res.ok) throw new Error((await res.text()) || "deliver_failed");
      return (await res.json()) as TransportSheet;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["transport-sheets"] });
      void qc.invalidateQueries({ queryKey: ["slips"] });
      void qc.invalidateQueries({ queryKey: ["parcels"] });
    },
  });

  function openPrint(sheet: TransportSheet) {
    void (async () => {
      let full = sheet;
      if (!sheet.sections) {
        const res = await apiFetch(`/inventory/transport-sheets/${sheet.id}`);
        if (res.ok) full = (await res.json()) as TransportSheet;
      }
      setPrintSheet(full);
      await markPrinted.mutateAsync(full.id);
      window.setTimeout(() => window.print(), 200);
    })();
  }

  function updateSection(idx: number, patch: Partial<DraftSection>) {
    setSections((prev) => {
      const next = [...prev];
      next[idx] = { ...next[idx], ...patch };
      return next;
    });
  }

  function toggleSlip(idx: number, slipId: string) {
    setSections((prev) => {
      const next = [...prev];
      const cur = new Set(next[idx].slip_ids);
      if (cur.has(slipId)) cur.delete(slipId);
      else cur.add(slipId);
      next[idx] = { ...next[idx], slip_ids: [...cur] };
      return next;
    });
  }

  const deptOptions = departments.data ?? [];
  const slipOptions = slips.data ?? [];

  return (
    <section className="panel transport-page">
      <h1>{t("trTitle")}</h1>
      <p className="muted">{t("trSubtitle")}</p>
      <p className="muted tip">{t("trTip")}</p>

      {canCreate ? (
        <form
          className="slip-form"
          onSubmit={(e) => {
            e.preventDefault();
            if (!selectedTo) return;
            create.mutate();
          }}
        >
          <div className="filter-bar">
            <label className="filter-field">
              <span>{t("trFrom")}</span>
              <strong>{labelBranch(fromBranch, locale)}</strong>
            </label>
            <label className="filter-field">
              <span>{t("trTo")}</span>
              <select value={selectedTo} onChange={(e) => setToBranch(e.target.value)} required>
                {otherBranches.length === 0 ? <option value="">{t("trNoOtherBranch")}</option> : null}
                {otherBranches.map((b) => (
                  <option key={b} value={b}>
                    {labelBranch(b, locale)}
                  </option>
                ))}
              </select>
            </label>
            <label className="filter-field">
              <span>{t("trCarrier")}</span>
              <input value={carrier} onChange={(e) => setCarrier(e.target.value)} placeholder={t("trCarrierPh")} />
            </label>
            <label className="filter-field">
              <span>{t("trVehicle")}</span>
              <input value={vehicle} onChange={(e) => setVehicle(e.target.value)} />
            </label>
            <label className="filter-field">
              <span>{t("trDriver")}</span>
              <input value={driver} onChange={(e) => setDriver(e.target.value)} />
            </label>
            <label className="filter-field">
              <span>{t("parcelColKind")}</span>
              <select value={parcelKind} onChange={(e) => setParcelKind(e.target.value)}>
                {PARCEL_KINDS.map((c) => (
                  <option key={c.code} value={c.code}>
                    {t(c.key)}
                  </option>
                ))}
              </select>
            </label>
          </div>

          <label className="filter-field block">
            <span>{t("trNotes")}</span>
            <input value={notes} onChange={(e) => setNotes(e.target.value)} placeholder={t("trNotesPh")} />
          </label>

          <h2>{t("trSections")}</h2>
          <p className="muted tip">{t("trSectionsTip")}</p>

          {sections.map((sec, idx) => (
            <div className="transport-section-draft" key={sec.key}>
              <div className="filter-bar">
                <label className="filter-field">
                  <span>{t("trDepartment")}</span>
                  <select
                    value={sec.department_code}
                    onChange={(e) => updateSection(idx, { department_code: e.target.value })}
                    required
                  >
                    <option value="">{t("trPickDept")}</option>
                    {deptOptions.map((d) => (
                      <option key={d.code} value={d.code}>
                        {d.name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="filter-field grow">
                  <span>{t("trDeptNotes")}</span>
                  <input
                    value={sec.notes}
                    onChange={(e) => updateSection(idx, { notes: e.target.value })}
                    placeholder={t("trDeptNotesPh")}
                  />
                </label>
              </div>
              {slipOptions.length > 0 ? (
                <div className="transport-slip-pick">
                  <span className="muted">{t("trAttachSlips")}</span>
                  <div className="transport-slip-chips">
                    {slipOptions.map((sl) => {
                      const on = sec.slip_ids.includes(sl.id);
                      return (
                        <button
                          key={sl.id}
                          type="button"
                          className={on ? "chip on" : "chip"}
                          onClick={() => toggleSlip(idx, sl.id)}
                        >
                          {sl.slip_number} · {sl.container_label || sl.container_type}
                        </button>
                      );
                    })}
                  </div>
                </div>
              ) : null}
            </div>
          ))}

          <div className="slip-form-actions">
            <button type="button" className="btn secondary" onClick={() => setSections((v) => [...v, newSection()])}>
              {t("trAddSection")}
            </button>
            <button type="submit" className="btn" disabled={create.isPending || !selectedTo}>
              {create.isPending ? t("trCreating") : t("trCreate")}
            </button>
          </div>
          {create.isError ? (
            <p className="error">{friendlyApiError((create.error as Error).message, locale)}</p>
          ) : null}
          {create.isSuccess ? <p className="muted tip">{t("trCreateOk")}</p> : null}
        </form>
      ) : null}

      <h2>{t("trHistory")}</h2>
      {list.isLoading ? <p className="muted">{t("trLoading")}</p> : null}
      {list.isError ? <p className="error">{t("trListError")}</p> : null}
      {(list.data ?? []).length === 0 && !list.isLoading ? <p className="muted">{t("trEmpty")}</p> : null}
      {(list.data ?? []).length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>{t("trColNumber")}</th>
              <th>{t("trColTo")}</th>
              <th>{t("parcelColKind")}</th>
              <th>{t("trColStatus")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {(list.data ?? []).map((s) => (
              <tr key={s.id}>
                <td>{s.sheet_number}</td>
                <td>{labelBranch(s.to_branch_id, locale)}</td>
                <td>{s.parcel_kind_label || s.parcel_kind || "TRANSFER"}</td>
                <td>{s.status}</td>
                <td className="slip-row-actions">
                  <button type="button" className="btn secondary" onClick={() => openPrint(s)}>
                    {t("trPrint")}
                  </button>
                  {canDepart && (s.status === "DRAFT" || s.status === "PRINTED") ? (
                    <button type="button" className="btn secondary" onClick={() => departSheet.mutate(s.id)}>
                      {t("trDepart")}
                    </button>
                  ) : null}
                  {canDeliver && s.status === "IN_TRANSIT" ? (
                    <button type="button" className="btn secondary" onClick={() => deliverSheet.mutate(s.id)}>
                      {t("trDeliver")}
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}

      {printSheet ? (
        <div className="transport-print-sheet" aria-label={t("trPrintSheet")}>
          <div className="slip-print-actions no-print">
            <button
              type="button"
              className="btn"
              onClick={() => {
                if (printSheet.sections == null) {
                  void apiFetch(`/inventory/transport-sheets/${printSheet.id}`)
                    .then((r) => r.json())
                    .then((full: TransportSheet) => {
                      setPrintSheet(full);
                      window.setTimeout(() => window.print(), 100);
                    });
                } else {
                  window.print();
                }
              }}
            >
              {t("trPrint")}
            </button>
            <button type="button" className="btn secondary" onClick={() => setPrintSheet(null)}>
              {t("trClosePreview")}
            </button>
          </div>
          <TransportPrintPreview sheet={printSheet} />
        </div>
      ) : null}
    </section>
  );
}

function TransportPrintPreview({ sheet }: { sheet: TransportSheet }) {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);

  // Ensure full detail when printing from list row.
  const detail = useQuery({
    queryKey: ["transport-sheet", sheet.id],
    enabled: !sheet.sections,
    queryFn: async () => {
      const res = await apiFetch(`/inventory/transport-sheets/${sheet.id}`);
      if (!res.ok) throw new Error("lookup_failed");
      return (await res.json()) as TransportSheet;
    },
  });

  const data = sheet.sections ? sheet : detail.data ?? sheet;
  const sections = data.sections ?? [];

  return (
    <article className="hoja-transporte">
      <header>
        <p className="papeleta-kicker">{t("trPrintSheet")}</p>
        <h2>{data.sheet_number}</h2>
      </header>
      <dl className="papeleta-meta">
        <div>
          <dt>{t("trFrom")}</dt>
          <dd>{labelBranch(data.from_branch_id, locale)}</dd>
        </div>
        <div>
          <dt>{t("trTo")}</dt>
          <dd>{labelBranch(data.to_branch_id, locale)}</dd>
        </div>
        {data.carrier_name ? (
          <div>
            <dt>{t("trCarrier")}</dt>
            <dd>{data.carrier_name}</dd>
          </div>
        ) : null}
        {data.vehicle_ref ? (
          <div>
            <dt>{t("trVehicle")}</dt>
            <dd>{data.vehicle_ref}</dd>
          </div>
        ) : null}
        {data.driver_name ? (
          <div>
            <dt>{t("trDriver")}</dt>
            <dd>{data.driver_name}</dd>
          </div>
        ) : null}
      </dl>
      {data.notes ? <p className="papeleta-desc">{data.notes}</p> : null}

      {sections.map((sec) => (
        <section className="hoja-dept" key={sec.id ?? sec.department_code}>
          <h3>{sec.department_name || sec.department_code}</h3>
          {sec.notes ? <p>{sec.notes}</p> : null}
          {(sec.slips ?? []).length > 0 ? (
            <ul className="papeleta-lines">
              {sec.slips!.map((sl) => (
                <li key={sl.id}>
                  <strong>{sl.slip_number}</strong> · {sl.container_label || sl.container_type} — {sl.description}
                </li>
              ))}
            </ul>
          ) : null}
        </section>
      ))}

      {data.operator_label ? (
        <p className="muted">
          {t("trOperator")}: {data.operator_label}
        </p>
      ) : null}
      <footer className="papeleta-footer">{t("trCarryHint")}</footer>
    </article>
  );
}
