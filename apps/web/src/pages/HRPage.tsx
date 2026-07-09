import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import {
  friendlyApiError,
  labelBranch,
  labelStatus,
  labelUser,
  useLocaleStore,
} from "../i18n/locale";

type Employee = {
  id: string;
  branch_id: string;
  employee_number: string;
  display_name: string;
  status: string;
};

type PayrollLine = {
  employee_id: string;
  employee_name?: string;
  concept_code: string;
  amount: number;
};

type PayrollRun = {
  id: string;
  period_label: string;
  branch_id: string;
  status: string;
  prepared_by: string;
  approved_by?: string;
  total_amount: number;
  version: number;
  lines?: PayrollLine[];
};

const hrNode = {
  id: "nav.hr",
  label: "Recursos Humanos",
  path: "/hr",
  require: {
    permissions: ["employee.read", "payroll.run.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function HRPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={hrNode}
      fallback={
        <section className="panel">
          <h1>{t("hrTitle")}</h1>
          <p className="error">{t("hrForbidden")}</p>
        </section>
      }
    >
      <HRPanel />
    </PolicyGuard>
  );
}

function HRPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const qc = useQueryClient();
  const canPrepare = hasPermission(claims, "payroll.run.prepare");
  const canApprove = hasPermission(claims, "payroll.run.approve");
  const canPayroll = hasPermission(claims, "payroll.run.read");
  const [tab, setTab] = useState<"employees" | "payroll">("employees");
  const [selectedRun, setSelectedRun] = useState<string | null>(null);

  const employees = useQuery({
    queryKey: ["employees", activeBranchId],
    queryFn: async () => {
      const qs = activeBranchId ? `?branch_id=${encodeURIComponent(activeBranchId)}` : "";
      const res = await apiFetch(`/payroll/employees${qs}`);
      if (!res.ok) throw new Error("employees_failed");
      const body = (await res.json()) as { items: Employee[] };
      return body.items ?? [];
    },
  });

  const runs = useQuery({
    queryKey: ["payroll-runs"],
    enabled: canPayroll && tab === "payroll",
    queryFn: async () => {
      const res = await apiFetch("/payroll/runs");
      if (!res.ok) throw new Error("runs_failed");
      return (await res.json()) as PayrollRun[];
    },
  });

  const runDetail = useQuery({
    queryKey: ["payroll-run", selectedRun],
    enabled: Boolean(selectedRun),
    queryFn: async () => {
      const res = await apiFetch(`/payroll/runs/${selectedRun}`);
      if (!res.ok) throw new Error("run_failed");
      return (await res.json()) as PayrollRun;
    },
  });

  const createRun = useMutation({
    mutationFn: async () => {
      const res = await apiFetch("/payroll/runs", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          period_label: "2026-07-H1",
          branch_id: activeBranchId,
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["payroll-runs"] });
    },
  });

  const approve = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/payroll/runs/${id}/approve`, { method: "POST" });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["payroll-runs"] });
      void qc.invalidateQueries({ queryKey: ["payroll-run"] });
    },
  });

  return (
    <section className="panel">
      <h1>{t("hrTitle")}</h1>
      <p className="muted">{t("hrSubtitle")}</p>

      <div className="recv-mode" role="tablist">
        <button type="button" className={tab === "employees" ? "active" : undefined} onClick={() => setTab("employees")}>
          {t("hrTabEmployees")}
        </button>
        {canPayroll ? (
          <button type="button" className={tab === "payroll" ? "active" : undefined} onClick={() => setTab("payroll")}>
            {t("hrTabPayroll")}
          </button>
        ) : null}
      </div>

      {tab === "employees" ? (
        <>
          <p className="muted tip">{t("hrEmployeesTip")}</p>
          {employees.isLoading ? <p className="muted">{t("hrLoadingEmployees")}</p> : null}
          {employees.data && employees.data.length === 0 ? <p className="muted">{t("hrNoEmployees")}</p> : null}
          {employees.data && employees.data.length > 0 ? (
            <table>
              <thead>
                <tr>
                  <th>{t("hrColNumber")}</th>
                  <th>{t("hrColName")}</th>
                  <th>{t("hrColBranch")}</th>
                  <th>{t("hrColStatus")}</th>
                </tr>
              </thead>
              <tbody>
                {employees.data.map((e) => (
                  <tr key={e.id}>
                    <td>{e.employee_number}</td>
                    <td>{e.display_name}</td>
                    <td>{labelBranch(e.branch_id, locale)}</td>
                    <td>
                      <span className="badge">{e.status}</span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
          {employees.isError ? (
            <p className="error">{friendlyApiError((employees.error as Error).message, locale)}</p>
          ) : null}
        </>
      ) : (
        <>
          <p className="muted tip">{t("paySubtitle")}</p>
          <div className="toolbar">
            <button
              type="button"
              className="btn"
              disabled={!canPrepare || !activeBranchId || createRun.isPending}
              onClick={() => createRun.mutate()}
            >
              {createRun.isPending ? t("payPreparing") : t("payPrepare")}
            </button>
            <Link className="btn secondary" to="/payroll">
              {t("hrOpenPayroll")}
            </Link>
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
                    <td style={{ display: "flex", gap: "0.35rem" }}>
                      <button type="button" className="btn secondary" onClick={() => setSelectedRun(r.id)}>
                        {t("hrViewLines")}
                      </button>
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

          {selectedRun && runDetail.data ? (
            <div style={{ marginTop: "1.25rem" }}>
              <h2>
                {t("hrRunLines")} — {runDetail.data.period_label}
              </h2>
              <table>
                <thead>
                  <tr>
                    <th>{t("hrColNumber")}</th>
                    <th>{t("hrColName")}</th>
                    <th>{t("hrColConcept")}</th>
                    <th>{t("payColTotal")}</th>
                  </tr>
                </thead>
                <tbody>
                  {(runDetail.data.lines ?? []).map((l, i) => (
                    <tr key={`${l.employee_id}-${l.concept_code}-${i}`}>
                      <td>{l.employee_id}</td>
                      <td>{l.employee_name || "—"}</td>
                      <td>{l.concept_code}</td>
                      <td>
                        {l.amount.toLocaleString(locale === "en" ? "en-US" : "es-MX", {
                          style: "currency",
                          currency: "MXN",
                        })}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}

          {createRun.isError ? (
            <p className="error">{friendlyApiError((createRun.error as Error).message, locale)}</p>
          ) : null}
          {approve.isError ? (
            <p className="error">{friendlyApiError((approve.error as Error).message, locale)}</p>
          ) : null}
        </>
      )}
    </section>
  );
}
