import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { NAV_NODES, hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type ApprovalRequest = {
  id: string;
  action_code: string;
  resource_type: string;
  resource_id: string;
  branch_id: string;
  warehouse_id: string;
  summary: string;
  status: string;
  requested_by_sub: string;
  requested_by_operator: string;
  requested_at: string;
};

const approvalsNode = NAV_NODES.find((n) => n.id === "nav.approvals")!;

export function ApprovalsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={approvalsNode}
      fallback={
        <section className="panel">
          <h1>{t("apprTitle")}</h1>
          <p className="error">{t("apprForbidden")}</p>
        </section>
      }
    >
      <ApprovalsPanel />
    </PolicyGuard>
  );
}

function ApprovalsPanel() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const qc = useQueryClient();
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const canDecide = hasPermission(claims, "approval.decide");

  const pending = useQuery({
    queryKey: ["approvals", activeBranchId],
    queryFn: async () => {
      const res = await apiFetch("/approvals");
      if (!res.ok) throw new Error("approvals_failed");
      return (await res.json()) as ApprovalRequest[];
    },
    refetchInterval: 8000,
  });

  const decide = useMutation({
    mutationFn: async (args: { id: string; approve: boolean }) => {
      const reason =
        locale === "en"
          ? args.approve
            ? "Approved by supervisor"
            : "Rejected by supervisor"
          : args.approve
            ? "Aprobado por jefe"
            : "Rechazado por jefe";
      const res = await apiFetch(`/approvals/${args.id}/decide`, {
        method: "POST",
        body: JSON.stringify({ approve: args.approve, reason }),
      });
      if (!res.ok) {
        const body = await res.text();
        throw new Error(body || "decide_failed");
      }
      return res.json();
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["approvals"] });
      void qc.invalidateQueries({ queryKey: ["movements"] });
      void qc.invalidateQueries({ queryKey: ["balances"] });
    },
  });

  const rows = pending.data ?? [];

  return (
    <section className="panel">
      <h1>{t("apprTitle")}</h1>
      <p className="muted">{t("apprSubtitle")}</p>
      {pending.isLoading ? <p className="muted">{t("apprLoading")}</p> : null}
      {pending.isError ? <p className="error">{t("apprError")}</p> : null}
      {!pending.isLoading && rows.length === 0 ? <p className="muted">{t("apprEmpty")}</p> : null}
      {rows.length > 0 ? (
        <ul className="approval-list">
          {rows.map((r) => (
            <li key={r.id} className="approval-item">
              <div>
                <strong>{r.summary || r.action_code}</strong>
                <p className="muted">
                  {r.requested_by_operator
                    ? t("apprRequestedByOp").replace("{op}", r.requested_by_operator)
                    : t("apprRequestedBy").replace("{sub}", r.requested_by_sub)}
                  {" · "}
                  {r.warehouse_id || r.branch_id}
                </p>
              </div>
              {canDecide ? (
                <div className="approval-actions">
                  <button
                    type="button"
                    className="btn"
                    disabled={decide.isPending}
                    onClick={() => decide.mutate({ id: r.id, approve: true })}
                  >
                    {t("apprApprove")}
                  </button>
                  <button
                    type="button"
                    className="btn secondary"
                    disabled={decide.isPending}
                    onClick={() => decide.mutate({ id: r.id, approve: false })}
                  >
                    {t("apprReject")}
                  </button>
                </div>
              ) : (
                <span className="badge">{t("apprWaiting")}</span>
              )}
            </li>
          ))}
        </ul>
      ) : null}
      {decide.isError ? (
        <p className="error">{friendlyApiError((decide.error as Error).message, locale)}</p>
      ) : null}
      {decide.isSuccess ? <p className="muted tip">{t("apprDecideSuccess")}</p> : null}
    </section>
  );
}
