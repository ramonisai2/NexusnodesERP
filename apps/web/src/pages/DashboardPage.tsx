import { labelOrg, labelPermission, useLocaleStore } from "../i18n/locale";
import { useAuthStore } from "../auth/store";

export function DashboardPage() {
  const claims = useAuthStore((s) => s.claims);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);

  return (
    <section className="panel">
      <h1>{t("dashTitle")}</h1>
      <p className="muted">{t("dashSubtitle")}</p>
      <div className="grid stats" style={{ marginTop: "1.25rem" }}>
        <div className="stat">
          <span className="muted">{t("dashOrg")}</span>
          <strong>{claims?.org_id ? labelOrg(claims.org_id, locale) : "—"}</strong>
        </div>
        <div className="stat">
          <span className="muted">{t("dashPermissions")}</span>
          <strong>{claims?.permissions.length ?? 0}</strong>
        </div>
        <div className="stat">
          <span className="muted">{t("dashBranches")}</span>
          <strong>{claims?.branch_ids.length ?? 0}</strong>
        </div>
      </div>
      <h2>{t("dashAccessTitle")}</h2>
      {(claims?.permissions?.length ?? 0) === 0 ? (
        <p className="muted">{t("dashEmptyAccess")}</p>
      ) : (
        <ul className="access-list">
          {(claims?.permissions ?? []).map((p) => (
            <li key={p}>{labelPermission(p, locale)}</li>
          ))}
        </ul>
      )}
      <p className="muted tip">{t("dashTip")}</p>
    </section>
  );
}
