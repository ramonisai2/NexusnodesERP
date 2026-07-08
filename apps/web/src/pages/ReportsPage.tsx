import { useQuery } from "@tanstack/react-query";
import { NAV_NODES } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { labelBranch, labelStatus, useLocaleStore } from "../i18n/locale";

type BranchDashboard = {
  branchId: string;
  inventory: {
    skuCount: number;
    totalOnHand: number;
    totalReserved: number;
    warehouseCount: number;
    topSkus: Array<{
      sku: string;
      productName: string;
      warehouseId: string;
      onHand: number;
    }>;
  };
  payroll: {
    runCount: number;
    inReviewCount: number;
    approvedCount: number;
    totalAmount: number;
    runs: Array<{
      id: string;
      periodLabel: string;
      status: string;
      totalAmount: number;
    }>;
  };
  byDepartment: Array<{
    departmentCode: string;
    departmentName: string;
    skuCount: number;
    totalOnHand: number;
  }>;
  labels: Array<{
    storeDisplayName: string;
    sku: string;
    materialCode: string;
    barcode: string;
    sizeCode?: string;
    colorCode?: string;
    publicDescription: string;
    departmentLabel?: string;
    priceMode: string;
    effectivePrice?: number;
    currency: string;
  }>;
};

const DASHBOARD_QUERY = `
query BranchDashboard($branchId: String!) {
  branchDashboard(branchId: $branchId) {
    branchId
    inventory {
      skuCount
      totalOnHand
      totalReserved
      warehouseCount
      topSkus { sku productName warehouseId onHand }
    }
    payroll {
      runCount
      inReviewCount
      approvedCount
      totalAmount
      runs { id periodLabel status totalAmount }
    }
    byDepartment {
      departmentCode
      departmentName
      skuCount
      totalOnHand
    }
    labels {
      storeDisplayName
      sku
      materialCode
      barcode
      sizeCode
      colorCode
      publicDescription
      departmentLabel
      priceMode
      effectivePrice
      currency
    }
  }
}`;

const reportsNode = NAV_NODES.find((n) => n.id === "nav.reports")!;

export function ReportsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={reportsNode}
      fallback={
        <section className="panel">
          <h1>{t("repTitle")}</h1>
          <p className="error">{t("repForbidden")}</p>
        </section>
      }
    >
      <ReportsPanel />
    </PolicyGuard>
  );
}

function ReportsPanel() {
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const branchId = activeBranchId || "br_norte";

  const dash = useQuery({
    queryKey: ["graphql-dashboard", branchId],
    queryFn: async () => {
      const res = await apiFetch("/graphql", {
        method: "POST",
        body: JSON.stringify({
          query: DASHBOARD_QUERY,
          variables: { branchId },
        }),
      });
      const body = await res.json();
      if (!res.ok || body.errors?.length) {
        throw new Error(body.errors?.[0]?.message || "graphql_failed");
      }
      return body.data.branchDashboard as BranchDashboard;
    },
  });

  function money(amount: number | undefined, currency = "MXN"): string {
    if (amount == null) return "—";
    try {
      return new Intl.NumberFormat(locale === "en" ? "en-US" : "es-MX", {
        style: "currency",
        currency,
      }).format(amount);
    } catch {
      return `${currency} ${amount.toFixed(2)}`;
    }
  }

  function priceMode(mode: string): string {
    if (mode === "SPECIAL") return t("priceModeSpecial");
    if (mode === "FINAL") return t("priceModeFinal");
    return t("priceModeCommon");
  }

  return (
    <section className="panel">
      <h1>{t("repTitle")}</h1>
      <p className="muted">{t("repSubtitle")}</p>
      <p className="muted tip">
        {t("repBranch")}: <strong>{labelBranch(branchId, locale)}</strong> · GraphQL BFF
      </p>

      {dash.isLoading ? <p className="muted">{t("repLoading")}</p> : null}
      {dash.isError ? <p className="error">{t("repError")}</p> : null}

      {dash.data ? (
        <>
          <div className="grid stats" style={{ marginTop: "1.25rem" }}>
            <div className="stat">
              <span className="muted">{t("repSkuCount")}</span>
              <strong>{dash.data.inventory.skuCount}</strong>
            </div>
            <div className="stat">
              <span className="muted">{t("repOnHand")}</span>
              <strong>{dash.data.inventory.totalOnHand}</strong>
            </div>
            <div className="stat">
              <span className="muted">{t("repPayrollRuns")}</span>
              <strong>{dash.data.payroll.runCount}</strong>
            </div>
            <div className="stat">
              <span className="muted">{t("repPayrollTotal")}</span>
              <strong>{money(dash.data.payroll.totalAmount)}</strong>
            </div>
          </div>

          <h2>{t("repByDept")}</h2>
          {dash.data.byDepartment.length === 0 ? (
            <p className="muted">{t("repNoDept")}</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>{t("invFilterDept")}</th>
                  <th>{t("repSkuCount")}</th>
                  <th>{t("repOnHand")}</th>
                </tr>
              </thead>
              <tbody>
                {dash.data.byDepartment.map((d) => (
                  <tr key={d.departmentCode}>
                    <td>{d.departmentName}</td>
                    <td>{d.skuCount}</td>
                    <td>{d.totalOnHand}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <h2 style={{ marginTop: "2rem" }}>{t("repLabels")}</h2>
          {dash.data.labels.length === 0 ? (
            <p className="muted">{t("invLabelsEmpty")}</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>{t("invColName")}</th>
                  <th>{t("invColMaterial")}</th>
                  <th>{t("invColBarcode")}</th>
                  <th>{t("invColSize")}</th>
                  <th>{t("invColColor")}</th>
                  <th>{t("priceEffective")}</th>
                </tr>
              </thead>
              <tbody>
                {dash.data.labels.map((l) => (
                  <tr key={`${l.sku}-${l.storeDisplayName}`}>
                    <td>
                      <div>{l.publicDescription}</div>
                      <div className="muted" style={{ fontSize: "0.85rem" }}>
                        {l.storeDisplayName} · {priceMode(l.priceMode)}
                      </div>
                    </td>
                    <td>{l.materialCode}</td>
                    <td>{l.barcode}</td>
                    <td>{l.sizeCode || "—"}</td>
                    <td>{l.colorCode || "—"}</td>
                    <td>{money(l.effectivePrice, l.currency)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          <h2 style={{ marginTop: "2rem" }}>{t("repPayroll")}</h2>
          {dash.data.payroll.runs.length === 0 ? (
            <p className="muted">{t("payNoRows")}</p>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>{t("payColPeriod")}</th>
                  <th>{t("payColStatus")}</th>
                  <th>{t("payColTotal")}</th>
                </tr>
              </thead>
              <tbody>
                {dash.data.payroll.runs.map((r) => (
                  <tr key={r.id}>
                    <td>{r.periodLabel}</td>
                    <td>{labelStatus(r.status, locale)}</td>
                    <td>{money(r.totalAmount)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      ) : null}
    </section>
  );
}
