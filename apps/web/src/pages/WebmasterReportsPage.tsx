import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { NAV_NODES, hasPermission, type SessionClaims } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { labelBranch, useLocaleStore } from "../i18n/locale";

export type ReportKind =
  | "inventory_balances"
  | "inventory_movements"
  | "inventory_adjustments"
  | "receipts"
  | "transfers"
  | "slips"
  | "transport"
  | "parcels"
  | "warranty"
  | "returns"
  | "pos_sales"
  | "customers"
  | "employees"
  | "payroll_runs"
  | "security_logistics"
  | "labels"
  | "catalog";

type ReportDef = {
  id: ReportKind;
  titleKey: string;
  hintKey: string;
  permissions: string[];
  path: (branchId: string) => string;
  rowsOf: (data: unknown) => Record<string, unknown>[];
};

function asItems(data: unknown): Record<string, unknown>[] {
  if (Array.isArray(data)) return data as Record<string, unknown>[];
  if (data && typeof data === "object") {
    const obj = data as Record<string, unknown>;
    for (const key of ["items", "rows", "employees", "runs", "sales", "customers"]) {
      if (Array.isArray(obj[key])) return obj[key] as Record<string, unknown>[];
    }
  }
  return [];
}

const REPORT_DEFS: ReportDef[] = [
  {
    id: "inventory_balances",
    titleKey: "wmRepBalances",
    hintKey: "wmRepBalancesHint",
    permissions: ["inventory.balance.read"],
    path: (b) => `/inventory/balances?branch_id=${encodeURIComponent(b)}&limit=500`,
    rowsOf: asItems,
  },
  {
    id: "inventory_movements",
    titleKey: "wmRepMovements",
    hintKey: "wmRepMovementsHint",
    permissions: ["inventory.movement.read"],
    path: (b) => `/inventory/movements?branch_id=${encodeURIComponent(b)}&limit=500`,
    rowsOf: asItems,
  },
  {
    id: "inventory_adjustments",
    titleKey: "wmRepAdjustments",
    hintKey: "wmRepAdjustmentsHint",
    permissions: ["inventory.adjustment.read", "inventory.movement.read"],
    path: (b) => `/inventory/movements?branch_id=${encodeURIComponent(b)}&limit=500`,
    rowsOf: (data) =>
      asItems(data).filter((r) => {
        const code = String(r.reason_code ?? r.reasonCode ?? "").toUpperCase();
        return ["MERMA", "ROBO", "DAMAGE", "EXPIRED", "COUNT_VARIANCE", "FOUND", "OTHER"].includes(code);
      }),
  },
  {
    id: "receipts",
    titleKey: "wmRepReceipts",
    hintKey: "wmRepReceiptsHint",
    permissions: ["inventory.receipt.read"],
    path: (b) => `/inventory/receipts?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "transfers",
    titleKey: "wmRepTransfers",
    hintKey: "wmRepTransfersHint",
    permissions: ["inventory.transfer.read"],
    path: (b) => `/inventory/transfers?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "slips",
    titleKey: "wmRepSlips",
    hintKey: "wmRepSlipsHint",
    permissions: ["inventory.slip.read"],
    path: (b) => `/inventory/slips?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "transport",
    titleKey: "wmRepTransport",
    hintKey: "wmRepTransportHint",
    permissions: ["inventory.transport.read"],
    path: (b) => `/inventory/transport-sheets?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "parcels",
    titleKey: "wmRepParcels",
    hintKey: "wmRepParcelsHint",
    permissions: ["inventory.parcel.read"],
    path: (b) => `/inventory/parcels?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "warranty",
    titleKey: "wmRepWarranty",
    hintKey: "wmRepWarrantyHint",
    permissions: ["inventory.warranty.read"],
    path: (b) => `/inventory/warranty-cases?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "returns",
    titleKey: "wmRepReturns",
    hintKey: "wmRepReturnsHint",
    permissions: ["inventory.return.read"],
    path: (b) => `/inventory/return-cases?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "pos_sales",
    titleKey: "wmRepPos",
    hintKey: "wmRepPosHint",
    permissions: ["pos.sale.read"],
    path: (b) => `/pos/sales?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "customers",
    titleKey: "wmRepCustomers",
    hintKey: "wmRepCustomersHint",
    permissions: ["customer.read"],
    path: () => `/customers?limit=200`,
    rowsOf: asItems,
  },
  {
    id: "employees",
    titleKey: "wmRepEmployees",
    hintKey: "wmRepEmployeesHint",
    permissions: ["employee.read"],
    path: (b) => `/payroll/employees?branch_id=${encodeURIComponent(b)}`,
    rowsOf: asItems,
  },
  {
    id: "payroll_runs",
    titleKey: "wmRepPayroll",
    hintKey: "wmRepPayrollHint",
    permissions: ["payroll.run.read"],
    path: (b) => `/payroll/runs?branch_id=${encodeURIComponent(b)}`,
    rowsOf: asItems,
  },
  {
    id: "security_logistics",
    titleKey: "wmRepSecurity",
    hintKey: "wmRepSecurityHint",
    permissions: ["reporting.security.read", "inventory.transport.read"],
    path: (b) => `/inventory/security-logistics?branch_id=${encodeURIComponent(b)}&limit=200`,
    rowsOf: asItems,
  },
  {
    id: "labels",
    titleKey: "wmRepLabels",
    hintKey: "wmRepLabelsHint",
    permissions: ["inventory.label.read", "inventory.balance.read"],
    path: (b) => `/inventory/labels?branch_id=${encodeURIComponent(b)}`,
    rowsOf: asItems,
  },
  {
    id: "catalog",
    titleKey: "wmRepCatalog",
    hintKey: "wmRepCatalogHint",
    permissions: ["inventory.catalog.read", "inventory.balance.read"],
    path: (b) => `/inventory/catalog?branch_id=${encodeURIComponent(b)}`,
    rowsOf: asItems,
  },
];

function canSeeReport(claims: SessionClaims | null, def: ReportDef): boolean {
  if (!claims) return false;
  if (claims.roles.includes("platform_admin") || claims.roles.includes("webmaster")) return true;
  return def.permissions.some((p) => hasPermission(claims, p));
}

function flattenRow(row: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(row)) {
    if (v == null) {
      out[k] = "";
    } else if (typeof v === "object") {
      out[k] = JSON.stringify(v);
    } else {
      out[k] = String(v);
    }
  }
  return out;
}

function toCSV(rows: Record<string, unknown>[]): string {
  if (rows.length === 0) return "";
  const flat = rows.map(flattenRow);
  const cols = Array.from(new Set(flat.flatMap((r) => Object.keys(r))));
  const esc = (s: string) => `"${s.replaceAll('"', '""')}"`;
  const lines = [cols.map(esc).join(",")];
  for (const r of flat) {
    lines.push(cols.map((c) => esc(r[c] ?? "")).join(","));
  }
  return lines.join("\n");
}

function downloadText(filename: string, content: string, mime: string) {
  const blob = new Blob([content], { type: mime });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

const webmasterNode = {
  id: "nav.webmasterReports",
  label: "Webmaster reports",
  path: "/reports/webmaster",
  require: {
    permissions: ["reporting.catalog.read", "reporting.export", "reporting.print", "reporting.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function WebmasterReportsPage() {
  const t = useLocaleStore((s) => s.t);
  const node = NAV_NODES.find((n) => n.id === "nav.webmasterReports") ?? webmasterNode;
  return (
    <PolicyGuard
      node={node}
      fallback={
        <section className="panel">
          <h1>{t("wmTitle")}</h1>
          <p className="error">{t("wmForbidden")}</p>
        </section>
      }
    >
      <WebmasterReportsPanel />
    </PolicyGuard>
  );
}

function WebmasterReportsPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branchId = activeBranchId || "br_norte";
  const canExport = hasPermission(claims, "reporting.export") || !!claims?.roles.includes("webmaster");
  const canPrint = hasPermission(claims, "reporting.print") || !!claims?.roles.includes("webmaster");

  const available = useMemo(
    () => REPORT_DEFS.filter((d) => canSeeReport(claims, d)),
    [claims],
  );
  const [kind, setKind] = useState<ReportKind>(available[0]?.id ?? "inventory_balances");
  const def = available.find((d) => d.id === kind) ?? available[0];

  const report = useQuery({
    queryKey: ["webmaster-report", kind, branchId],
    enabled: !!def,
    queryFn: async () => {
      if (!def) throw new Error("no_report");
      const res = await apiFetch(def.path(branchId));
      if (!res.ok) throw new Error(`report_failed_${res.status}`);
      const data = await res.json();
      return def.rowsOf(data);
    },
  });

  const rows = report.data ?? [];
  const columns = useMemo(() => {
    if (rows.length === 0) return [] as string[];
    return Object.keys(flattenRow(rows[0]));
  }, [rows]);

  function onExportCSV() {
    if (!def || !canExport) return;
    const csv = toCSV(rows);
    const stamp = new Date().toISOString().slice(0, 19).replaceAll(":", "");
    downloadText(`nexus-${def.id}-${branchId}-${stamp}.csv`, csv, "text/csv;charset=utf-8");
  }

  function onExportJSON() {
    if (!def || !canExport) return;
    const stamp = new Date().toISOString().slice(0, 19).replaceAll(":", "");
    downloadText(
      `nexus-${def.id}-${branchId}-${stamp}.json`,
      JSON.stringify({ report: def.id, branch_id: branchId, generated_at: new Date().toISOString(), rows }, null, 2),
      "application/json",
    );
  }

  function onPrint() {
    if (!canPrint) return;
    window.print();
  }

  return (
    <section className="panel webmaster-reports">
      <div className="wm-toolbar no-print">
        <div>
          <h1>{t("wmTitle")}</h1>
          <p className="muted">{t("wmSubtitle")}</p>
          <p className="muted tip">
            {t("repBranch")}: <strong>{labelBranch(branchId, locale)}</strong>
          </p>
        </div>
        <div className="wm-actions">
          <button type="button" className="btn secondary" disabled={!canPrint || report.isLoading} onClick={onPrint}>
            {t("wmPrint")}
          </button>
          <button type="button" className="btn secondary" disabled={!canExport || rows.length === 0} onClick={onExportCSV}>
            {t("wmExportCsv")}
          </button>
          <button type="button" className="btn secondary" disabled={!canExport || rows.length === 0} onClick={onExportJSON}>
            {t("wmExportJson")}
          </button>
        </div>
      </div>

      <div className="wm-catalog no-print">
        <label htmlFor="wm-kind">
          {t("wmSelectReport")}
          <select
            id="wm-kind"
            value={def?.id ?? ""}
            onChange={(e) => setKind(e.target.value as ReportKind)}
          >
            {available.map((d) => (
              <option key={d.id} value={d.id}>
                {t(d.titleKey)}
              </option>
            ))}
          </select>
        </label>
        {def ? <p className="hint">{t(def.hintKey)}</p> : null}
      </div>

      <div className="wm-print-sheet" aria-label={t("wmPrintSheet")}>
        <header className="wm-print-header">
          <h2>{def ? t(def.titleKey) : t("wmTitle")}</h2>
          <p>
            NexusERP · {labelBranch(branchId, locale)} · {new Date().toLocaleString(locale === "en" ? "en-US" : "es-MX")}
          </p>
        </header>

        {report.isLoading ? <p className="muted no-print">{t("wmLoading")}</p> : null}
        {report.isError ? <p className="error no-print">{t("wmError")}</p> : null}

        {!report.isLoading && rows.length === 0 ? (
          <p className="muted">{t("wmEmpty")}</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  {columns.map((c) => (
                    <th key={c}>{c}</th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {rows.slice(0, 500).map((row, i) => {
                  const flat = flattenRow(row);
                  return (
                    <tr key={i}>
                      {columns.map((c) => (
                        <td key={c}>{flat[c]}</td>
                      ))}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
        <p className="muted tip">
          {t("wmRowCount")}: {rows.length}
          {rows.length > 500 ? ` (${t("wmPrintTruncated")})` : ""}
        </p>
      </div>
    </section>
  );
}
