import { Link } from "react-router-dom";
import { labelOrg, labelPermission, useLocaleStore } from "../i18n/locale";
import { useAuthStore } from "../auth/store";

export function DashboardPage() {
  const claims = useAuthStore((s) => s.claims);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const isDemo =
    claims?.attrs?.demo_mode === true ||
    claims?.sub === "usr_dev_demo";
  const isShop =
    !isDemo &&
    (claims?.roles?.includes("store_owner") ||
      claims?.attrs?.profile === "abarrotes" ||
      typeof claims?.attrs?.store_name === "string");

  const storeLabel =
    (typeof claims?.attrs?.store_name === "string" && claims.attrs.store_name) ||
    (claims?.org_id ? labelOrg(claims.org_id, locale) : "—");

  if (isDemo) {
    return (
      <section className="panel demo-home">
        <p className="setup-kicker">{t("demoDashKicker")}</p>
        <h1>{t("demoDashTitle")}</h1>
        <p className="muted">{t("demoDashSubtitle")}</p>
        <div className="shop-actions">
          <Link className="shop-tile" to="/inventory">
            <strong>{t("demoTileInventory")}</strong>
            <span className="muted">{t("demoTileInventoryHint")}</span>
          </Link>
          <Link className="shop-tile" to="/caja">
            <strong>{t("demoTilePos")}</strong>
            <span className="muted">{t("demoTilePosHint")}</span>
          </Link>
          <Link className="shop-tile" to="/inventory/receiving">
            <strong>{t("demoTileReceiving")}</strong>
            <span className="muted">{t("demoTileReceivingHint")}</span>
          </Link>
          <Link className="shop-tile" to="/reports">
            <strong>{t("demoTileReports")}</strong>
            <span className="muted">{t("demoTileReportsHint")}</span>
          </Link>
          <Link className="shop-tile" to="/hr">
            <strong>{t("demoTileHr")}</strong>
            <span className="muted">{t("demoTileHrHint")}</span>
          </Link>
          <Link className="shop-tile" to="/reports/seguridad">
            <strong>{t("demoTileSecurity")}</strong>
            <span className="muted">{t("demoTileSecurityHint")}</span>
          </Link>
        </div>
        <p className="muted tip">{t("demoDashTip")}</p>
      </section>
    );
  }

  if (isShop) {
    return (
      <section className="panel shop-home">
        <p className="setup-kicker">{t("dashShopKicker")}</p>
        <h1>{t("dashShopTitle")}</h1>
        <p className="muted">
          {t("dashShopSubtitle")} <strong>{storeLabel}</strong>
        </p>
        <div className="shop-actions">
          <Link className="shop-tile" to="/inventory">
            <strong>{t("dashShopInventory")}</strong>
            <span className="muted">{t("dashShopInventoryHint")}</span>
          </Link>
          <Link className="shop-tile" to="/reports/images">
            <strong>{t("dashShopPhotos")}</strong>
            <span className="muted">{t("dashShopPhotosHint")}</span>
          </Link>
          <Link className="shop-tile" to="/reports">
            <strong>{t("dashShopReports")}</strong>
            <span className="muted">{t("dashShopReportsHint")}</span>
          </Link>
        </div>
        <p className="muted tip">{t("dashShopTip")}</p>
      </section>
    );
  }

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
