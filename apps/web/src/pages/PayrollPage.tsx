import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";

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
  return (
    <PolicyGuard
      node={payrollNode}
      fallback={
        <section className="panel">
          <h1>Nómina</h1>
          <p className="error">No tienes permiso para ver nóminas en esta sesión.</p>
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
      <h1>Nómina</h1>
      <p className="muted">
        Ciclo con segregación de funciones: quien prepara no puede aprobar la misma corrida.
      </p>
      <div style={{ margin: "1rem 0", display: "flex", gap: "0.75rem" }}>
        <button
          type="button"
          className="btn"
          disabled={!canPrepare || !activeBranchId || createRun.isPending}
          onClick={() => createRun.mutate()}
        >
          Preparar corrida
        </button>
      </div>
      {runs.isLoading ? <p className="muted">Cargando…</p> : null}
      {runs.data ? (
        <table>
          <thead>
            <tr>
              <th>Periodo</th>
              <th>Sucursal</th>
              <th>Estado</th>
              <th>Total</th>
              <th>Preparó</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {runs.data.map((r) => (
              <tr key={r.id}>
                <td>{r.period_label}</td>
                <td>{r.branch_id}</td>
                <td>
                  <span className="badge">{r.status}</span>
                </td>
                <td>{r.total_amount.toLocaleString("es-MX")}</td>
                <td>{r.prepared_by}</td>
                <td>
                  <button
                    type="button"
                    className="btn secondary"
                    disabled={!canApprove || r.status !== "IN_REVIEW" || approve.isPending}
                    onClick={() => approve.mutate(r.id)}
                  >
                    Aprobar
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}
      {createRun.isError ? <p className="error">{(createRun.error as Error).message}</p> : null}
      {approve.isError ? <p className="error">{(approve.error as Error).message}</p> : null}
    </section>
  );
}
