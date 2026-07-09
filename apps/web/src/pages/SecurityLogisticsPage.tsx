import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { PolicyGuard } from "../auth/PolicyGuard";
import { hasPermission } from "../auth/policy";
import { apiFetch, useAuthStore } from "../auth/store";
import { friendlyApiError, labelBranch, useLocaleStore } from "../i18n/locale";

type SecurityEvent = {
  kind: "TRANSPORT" | "INBOUND" | "SLIP" | string;
  id: string;
  folio: string;
  branch_from?: string;
  branch_to?: string;
  branch_id?: string;
  status: string;
  carrier_name?: string;
  vehicle_ref?: string;
  driver_name?: string;
  dock_door?: string;
  seal_number?: string;
  seal_status?: string;
  seal_verified_at?: string;
  container_types?: string;
  event_at?: string;
  created_at: string;
  notes?: string;
};

const node = {
  id: "nav.security",
  label: "Seguridad",
  path: "/reports/seguridad",
  require: {
    permissions: ["reporting.security.read", "inventory.transport.read", "inventory.shipment.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function SecurityLogisticsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={node}
      fallback={
        <section className="panel">
          <h1>{t("secTitle")}</h1>
          <p className="error">{t("secForbidden")}</p>
        </section>
      }
    >
      <SecurityPanel />
    </PolicyGuard>
  );
}

function SecurityPanel() {
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const qc = useQueryClient();
  const canVerify = hasPermission(claims, "inventory.seal.verify");

  const [kind, setKind] = useState("");
  const [sealOnly, setSealOnly] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [verifyID, setVerifyID] = useState<string | null>(null);
  const [verifyKind, setVerifyKind] = useState<"TRANSPORT" | "INBOUND">("TRANSPORT");
  const [sealNumber, setSealNumber] = useState("");
  const [sealStatus, setSealStatus] = useState("VERIFIED");
  const [sealNotes, setSealNotes] = useState("");

  const report = useQuery({
    queryKey: ["security-logistics", activeBranchId, kind, sealOnly],
    queryFn: async () => {
      const qs = new URLSearchParams({ limit: "80" });
      if (activeBranchId) qs.set("branch_id", activeBranchId);
      if (kind) qs.set("kind", kind);
      if (sealOnly) qs.set("seal_only", "1");
      const res = await apiFetch(`/inventory/security-logistics?${qs}`);
      if (!res.ok) throw new Error("security_report_failed");
      const body = (await res.json()) as { items: SecurityEvent[] };
      return body.items ?? [];
    },
  });

  const verify = useMutation({
    mutationFn: async () => {
      if (!verifyID) throw new Error("missing_id");
      const path =
        verifyKind === "TRANSPORT"
          ? `/inventory/transport-sheets/${verifyID}/verify-seal`
          : `/inventory/inbound-shipments/${verifyID}/verify-seal`;
      const res = await apiFetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          seal_number: sealNumber.trim() || undefined,
          seal_status: sealStatus,
          notes: sealNotes.trim() || undefined,
        }),
      });
      const body = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(body.detail || body.error || "verify_failed");
      return body;
    },
    onSuccess: () => {
      setMessage(t("secVerifyOk"));
      setVerifyID(null);
      setSealNotes("");
      void qc.invalidateQueries({ queryKey: ["security-logistics"] });
    },
    onError: (err) => setMessage(friendlyApiError((err as Error).message, locale) || t("secVerifyError")),
  });

  const openVerify = (row: SecurityEvent) => {
    if (row.kind !== "TRANSPORT" && row.kind !== "INBOUND") return;
    setVerifyKind(row.kind);
    setVerifyID(row.id);
    setSealNumber(row.seal_number || "");
    setSealStatus(row.seal_status === "BROKEN" || row.seal_status === "MISSING" ? row.seal_status : "VERIFIED");
    setMessage(null);
  };

  return (
    <section className="panel">
      <h1>{t("secTitle")}</h1>
      <p className="muted">{t("secSubtitle")}</p>
      <p className="muted tip">{t("secTip")}</p>

      <div className="toolbar" style={{ flexWrap: "wrap", gap: "0.75rem" }}>
        <label>
          <span className="muted">{t("secFilterKind")}</span>
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="">{t("secAllKinds")}</option>
            <option value="TRANSPORT">{t("secKindTransport")}</option>
            <option value="INBOUND">{t("secKindInbound")}</option>
            <option value="SLIP">{t("secKindSlip")}</option>
          </select>
        </label>
        <label className="recv-check">
          <input type="checkbox" checked={sealOnly} onChange={(e) => setSealOnly(e.target.checked)} />
          <span>{t("secSealOnly")}</span>
        </label>
      </div>

      {report.isLoading ? <p className="muted">{t("secLoading")}</p> : null}
      {report.isError ? (
        <p className="error">{friendlyApiError((report.error as Error).message, locale)}</p>
      ) : null}
      {report.data && report.data.length === 0 ? <p className="muted">{t("secNoRows")}</p> : null}

      {report.data && report.data.length > 0 ? (
        <table>
          <thead>
            <tr>
              <th>{t("secColKind")}</th>
              <th>{t("secColFolio")}</th>
              <th>{t("secColRoute")}</th>
              <th>{t("secColVehicle")}</th>
              <th>{t("secColSeal")}</th>
              <th>{t("secColContainers")}</th>
              <th>{t("secColStatus")}</th>
              <th>{t("secColWhen")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {report.data.map((row) => (
              <tr key={`${row.kind}-${row.id}`}>
                <td>{kindLabel(row.kind, t)}</td>
                <td>
                  <code>{row.folio}</code>
                </td>
                <td>{routeLabel(row, locale)}</td>
                <td>
                  {[row.vehicle_ref, row.driver_name, row.carrier_name].filter(Boolean).join(" · ") || "—"}
                  {row.dock_door ? <span className="muted"> · {row.dock_door}</span> : null}
                </td>
                <td>
                  {row.seal_number ? (
                    <>
                      <strong>{row.seal_number}</strong>
                      {row.seal_status ? (
                        <span className="badge" style={{ marginLeft: "0.35rem" }}>
                          {sealStatusLabel(row.seal_status, t)}
                        </span>
                      ) : null}
                    </>
                  ) : (
                    <span className="muted">—</span>
                  )}
                </td>
                <td>{row.container_types || "—"}</td>
                <td>
                  <span className="badge">{row.status}</span>
                </td>
                <td>{row.event_at ? new Date(row.event_at).toLocaleString(locale === "en" ? "en-US" : "es-MX") : "—"}</td>
                <td>
                  {canVerify && (row.kind === "TRANSPORT" || row.kind === "INBOUND") ? (
                    <button type="button" className="btn secondary" onClick={() => openVerify(row)}>
                      {t("secVerify")}
                    </button>
                  ) : null}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      ) : null}

      {verifyID ? (
        <form
          className="recv-form"
          style={{ marginTop: "1.25rem" }}
          onSubmit={(e) => {
            e.preventDefault();
            verify.mutate();
          }}
        >
          <h2>{t("secVerifyTitle")}</h2>
          <div className="recv-grid">
            <label>
              <span className="muted">{t("secSealNumber")}</span>
              <input value={sealNumber} onChange={(e) => setSealNumber(e.target.value)} maxLength={40} />
            </label>
            <label>
              <span className="muted">{t("secSealStatus")}</span>
              <select value={sealStatus} onChange={(e) => setSealStatus(e.target.value)}>
                <option value="VERIFIED">{t("secSeal_VERIFIED")}</option>
                <option value="BROKEN">{t("secSeal_BROKEN")}</option>
                <option value="MISSING">{t("secSeal_MISSING")}</option>
                <option value="APPLIED">{t("secSeal_APPLIED")}</option>
              </select>
            </label>
          </div>
          <label>
            <span className="muted">{t("secSealNotes")}</span>
            <textarea value={sealNotes} onChange={(e) => setSealNotes(e.target.value)} rows={2} maxLength={400} />
          </label>
          <div className="toolbar">
            <button type="submit" className="btn" disabled={verify.isPending}>
              {verify.isPending ? t("secVerifying") : t("secConfirmVerify")}
            </button>
            <button type="button" className="btn secondary" onClick={() => setVerifyID(null)}>
              {t("secCancel")}
            </button>
          </div>
        </form>
      ) : null}

      {message ? <p className={verify.isError ? "error" : "ok"}>{message}</p> : null}
    </section>
  );
}

function kindLabel(kind: string, t: (k: "secKindTransport") => string) {
  if (kind === "TRANSPORT") return t("secKindTransport");
  if (kind === "INBOUND") return t("secKindInbound");
  if (kind === "SLIP") return t("secKindSlip");
  return kind;
}

function sealStatusLabel(status: string, t: (k: "secSeal_VERIFIED") => string) {
  const key = `secSeal_${status}` as "secSeal_VERIFIED";
  try {
    return t(key);
  } catch {
    return status;
  }
}

function routeLabel(row: SecurityEvent, locale: "es" | "en") {
  if (row.kind === "INBOUND") {
    return labelBranch(row.branch_id || "", locale) || row.branch_id || "—";
  }
  const from = labelBranch(row.branch_from || "", locale) || row.branch_from || "?";
  const to = labelBranch(row.branch_to || "", locale) || row.branch_to || "?";
  return `${from} → ${to}`;
}
