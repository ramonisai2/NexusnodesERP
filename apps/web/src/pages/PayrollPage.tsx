import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import {
  friendlyApiError,
  labelBranch,
  labelStatus,
  labelUser,
  useLocaleStore,
} from "../i18n/locale";

type PayrollRun = {
  id: string;
  period_label: string;
  branch_id: string;
  status: string;
  prepared_by: string;
  approved_by?: string;
  total_amount: number;
  version: number;
};

const payrollNode = NAV_NODES.find((n) => n.id === "nav.payroll")!;

export function PayrollPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={payrollNode}
      fallback={
        <section className="panel">
          <h1>{t("payTitle")}</h1>
          <p className="error">{t("payForbidden")}</p>
        </section>
      }
    >
      <PayrollPanel />
    </PolicyGuard>
  );
}

function PayrollPanel() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const qc = useQueryClient();
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const canPrepare = hasPermission(claims, "payroll.run.prepare");
  const canApprove = hasPermission(claims, "payroll.run.approve");

  const runs = useQuery({
    queryKey: ["payroll-runs"],
    queryFn: async () => {
      const res = await apiFetch("/payroll/runs");
      if (!res.ok) throw new Error("runs_failed");
      return (await res.json()) as PayrollRun[];
    },
  });

  const createRun = useMutation({
    mutationFn: async () => {
      const res = await apiFetch("/payroll/runs", {
        method: "POST",
        body: JSON.stringify({
          period_label: "2026-07-H1",
          branch_id: activeBranchId,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["payroll-runs"] }),
  });

  const approve = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/payroll/runs/${id}/approve`, { method: "POST" });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["payroll-runs"] }),
  });

  return (
    <section className="panel">
      <h1>{t("payTitle")}</h1>
      <p className="muted">{t("paySubtitle")}</p>
      <div className="toolbar">
        <button
          type="button"
          className="btn"
          disabled={!canPrepare || !activeBranchId || createRun.isPending}
          onClick={() => createRun.mutate()}
        >
          {createRun.isPending ? t("payPreparing") : t("payPrepare")}
        </button>
      </div>
      {runs.isLoading ? <p className="muted">{t("payLoading")}</p> : null}
      {runs.data && runs.data.length === 0 ? <p className="muted">{t("payNoRows")}</p> : null}
      {runs.data && runs.data.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>{t("payColPeriod")}</th>
              <th>{t("payColBranch")}</th>
              <th>{t("payColStatus")}</th>
              <th>{t("payColTotal")}</th>
              <th>{t("payColPreparedBy")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {runs.data.map((r) => (
              <tr key={r.id}>
                <td>{r.period_label}</td>
                <td>{labelBranch(r.branch_id, locale)}</td>
                <td>
                  <span className="badge">{labelStatus(r.status, locale)}</span>
                </td>
                <td>
                  {r.total_amount.toLocaleString(locale === "en" ? "en-US" : "es-MX", {
                    style: "currency",
                    currency: "MXN",
                  })}
                </td>
                <td>{labelUser(r.prepared_by)}</td>
                <td>
                  <button
                    type="button"
                    className="btn secondary"
                    disabled={!canApprove || r.status !== "IN_REVIEW" || approve.isPending}
                    onClick={() => approve.mutate(r.id)}
                  >
                    {approve.isPending ? t("payApproving") : t("payApprove")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {createRun.isError ? (
        <p className="error">{friendlyApiError((createRun.error as Error).message, locale)}</p>
      ) : null}
      {approve.isError ? (
        <p className="error">{friendlyApiError((approve.error as Error).message, locale)}</p>
      ) : null}
    </section>
  );
}
