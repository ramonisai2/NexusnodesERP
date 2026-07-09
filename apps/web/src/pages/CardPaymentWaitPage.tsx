import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PolicyGuard } from "../auth/PolicyGuard";
import { hasPermission, NAV_NODES } from "../auth/policy";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, useLocaleStore } from "../i18n/locale";

type WaitLine = {
  sku: string;
  description?: string;
  quantity: number;
  unit_price?: number;
  line_total?: number;
};

type CardWait = {
  id: string;
  branch_id: string;
  status: string;
  status_label?: string;
  amount: number;
  currency: string;
  customer_name?: string;
  lines: WaitLine[];
  terminal_ref?: string;
  auth_code?: string;
  decline_reason?: string;
  sale_id?: string;
  operator_label?: string;
  created_at: string;
  resolved_at?: string;
};

const cardWaitNode = NAV_NODES.find((n) => n.id === "nav.cardWaits")!;

function money(n: number) {
  return n.toLocaleString("es-MX", { style: "currency", currency: "MXN" });
}

export function CardPaymentWaitPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={cardWaitNode}
      fallback={
        <section className="panel">
          <h1>{t("cardWaitTitle")}</h1>
          <p className="error">{t("cardWaitForbidden")}</p>
        </section>
      }
    >
      <CardPaymentWaitPanel />
    </PolicyGuard>
  );
}

function CardPaymentWaitPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const branchId = useAuthStore((s) => s.activeBranchId) || claims?.branch_ids?.[0] || "";
  const canConfirm = hasPermission(claims, "pos.card.wait.confirm");
  const canCancel = hasPermission(claims, "pos.card.wait.cancel") || hasPermission(claims, "pos.card.wait.create");
  const qc = useQueryClient();

  const [statusFilter, setStatusFilter] = useState("WAITING");
  const [authCode, setAuthCode] = useState<Record<string, string>>({});
  const [terminalRef, setTerminalRef] = useState<Record<string, string>>({});
  const [declineReason, setDeclineReason] = useState<Record<string, string>>({});
  const [error, setError] = useState("");
  const [okMsg, setOkMsg] = useState("");

  const waits = useQuery({
    queryKey: ["pos-card-waits", branchId, statusFilter],
    enabled: Boolean(branchId),
    refetchInterval: statusFilter === "WAITING" ? 4000 : false,
    queryFn: async () => {
      const q = new URLSearchParams({ branch_id: branchId, limit: "40" });
      if (statusFilter) q.set("status", statusFilter);
      const res = await apiFetch(`/pos/card-waits?${q}`);
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as { items: CardWait[] };
    },
  });

  const items = useMemo(() => waits.data?.items || [], [waits.data]);

  const confirm = useMutation({
    mutationFn: async (args: { id: string; approved: boolean }) => {
      const res = await apiFetch(`/pos/card-waits/${args.id}/confirm`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          approved: args.approved,
          auth_code: authCode[args.id] || "",
          terminal_ref: terminalRef[args.id] || "",
          decline_reason: args.approved ? "" : declineReason[args.id] || t("cardWaitDeclinedDefault"),
        }),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as CardWait;
    },
    onSuccess: (item) => {
      setError("");
      setOkMsg(
        item.status === "APPROVED"
          ? t("cardWaitApprovedOk")
          : t("cardWaitDeclinedOk"),
      );
      void qc.invalidateQueries({ queryKey: ["pos-card-waits"] });
      void qc.invalidateQueries({ queryKey: ["pos-sales"] });
    },
    onError: (err: Error) => setError(friendlyApiError(err.message, locale) || t("cardWaitActionError")),
  });

  const cancel = useMutation({
    mutationFn: async (id: string) => {
      const res = await apiFetch(`/pos/card-waits/${id}/cancel`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ reason: declineReason[id] || t("cardWaitCancelDefault") }),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as CardWait;
    },
    onSuccess: () => {
      setError("");
      setOkMsg(t("cardWaitCancelledOk"));
      void qc.invalidateQueries({ queryKey: ["pos-card-waits"] });
    },
    onError: (err: Error) => setError(friendlyApiError(err.message, locale) || t("cardWaitActionError")),
  });

  return (
    <section className="panel">
      <header className="transfer-header">
        <div>
          <h1>{t("cardWaitTitle")}</h1>
          <p className="muted">{t("cardWaitSubtitle")}</p>
        </div>
        <Link className="btn secondary" to="/caja">
          {t("cardWaitBackPos")}
        </Link>
      </header>

      {error ? <p className="error">{error}</p> : null}
      {okMsg ? <p className="ok">{okMsg}</p> : null}

      <div className="toolbar-row" style={{ display: "flex", gap: "0.75rem", flexWrap: "wrap", marginBottom: "1rem" }}>
        <label>
          {t("cardWaitFilter")}
          <select value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="WAITING">{t("cardWaitStatusWaiting")}</option>
            <option value="APPROVED">{t("cardWaitStatusApproved")}</option>
            <option value="DECLINED">{t("cardWaitStatusDeclined")}</option>
            <option value="CANCELLED">{t("cardWaitStatusCancelled")}</option>
            <option value="">{t("cardWaitStatusAll")}</option>
          </select>
        </label>
        <button type="button" className="btn secondary" onClick={() => void waits.refetch()}>
          {t("cardWaitRefresh")}
        </button>
      </div>

      {waits.isLoading ? <p className="muted">{t("cardWaitLoading")}</p> : null}

      <ul className="plain-list card-wait-list">
        {items.map((w) => (
          <li key={w.id} className="card-wait-item">
            <div className="card-wait-head">
              <div>
                <strong>{money(w.amount)}</strong>{" "}
                <span className="muted">{w.currency}</span>
                <div className="muted">
                  {w.status_label || w.status} · {new Date(w.created_at).toLocaleString(locale === "en" ? "en-US" : "es-MX")}
                  {w.operator_label ? ` · ${w.operator_label}` : ""}
                </div>
              </div>
              {w.sale_id ? (
                <span className="ok">
                  {t("cardWaitSaleLinked")}: <code>{w.sale_id.slice(0, 8)}</code>
                </span>
              ) : null}
            </div>

            <ul className="muted" style={{ margin: "0.5rem 0", paddingLeft: "1.1rem" }}>
              {(w.lines || []).map((l, i) => (
                <li key={`${l.sku}-${i}`}>
                  {l.description || l.sku} · {l.quantity} × {money(l.unit_price || 0)}
                </li>
              ))}
            </ul>

            {w.status === "WAITING" ? (
              <div className="card-wait-actions" style={{ display: "grid", gap: "0.5rem", maxWidth: 420 }}>
                <label>
                  {t("cardWaitTerminalRef")}
                  <input
                    value={terminalRef[w.id] ?? w.terminal_ref ?? ""}
                    onChange={(e) => setTerminalRef((prev) => ({ ...prev, [w.id]: e.target.value }))}
                    placeholder={t("cardWaitTerminalPh")}
                  />
                </label>
                <label>
                  {t("cardWaitAuthCode")}
                  <input
                    value={authCode[w.id] || ""}
                    onChange={(e) => setAuthCode((prev) => ({ ...prev, [w.id]: e.target.value }))}
                    placeholder={t("cardWaitAuthPh")}
                  />
                </label>
                <label>
                  {t("cardWaitDeclineReason")}
                  <input
                    value={declineReason[w.id] || ""}
                    onChange={(e) => setDeclineReason((prev) => ({ ...prev, [w.id]: e.target.value }))}
                    placeholder={t("cardWaitDeclinePh")}
                  />
                </label>
                <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
                  {canConfirm ? (
                    <>
                      <button
                        type="button"
                        className="btn"
                        disabled={confirm.isPending}
                        onClick={() => confirm.mutate({ id: w.id, approved: true })}
                      >
                        {t("cardWaitApprove")}
                      </button>
                      <button
                        type="button"
                        className="btn secondary"
                        disabled={confirm.isPending}
                        onClick={() => confirm.mutate({ id: w.id, approved: false })}
                      >
                        {t("cardWaitDecline")}
                      </button>
                    </>
                  ) : null}
                  {canCancel ? (
                    <button
                      type="button"
                      className="btn secondary"
                      disabled={cancel.isPending}
                      onClick={() => cancel.mutate(w.id)}
                    >
                      {t("cardWaitCancel")}
                    </button>
                  ) : null}
                </div>
              </div>
            ) : (
              <p className="muted">
                {w.auth_code ? `${t("cardWaitAuthCode")}: ${w.auth_code}` : null}
                {w.decline_reason ? ` · ${w.decline_reason}` : null}
                {w.terminal_ref ? ` · ${w.terminal_ref}` : null}
              </p>
            )}
          </li>
        ))}
        {!waits.isLoading && items.length === 0 ? (
          <li className="muted">{t("cardWaitEmpty")}</li>
        ) : null}
      </ul>
    </section>
  );
}
