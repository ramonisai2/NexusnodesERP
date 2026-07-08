import { NavLink, Outlet } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { canAccess, type NavNode } from "../auth/policy";
import { useAuthStore } from "../auth/store";
import { labelBranch, labelRole, labelUser, useLocaleStore } from "../i18n/locale";

export function AppShell() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const setActiveBranch = useAuthStore((s) => s.setActiveBranch);
  const logout = useAuthStore((s) => s.logout);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const isShop = claims?.roles?.includes("store_owner") || claims?.attrs?.profile === "abarrotes";

  const nav: NavNode[] = [
    { id: "nav.dashboard", label: t("navHome"), path: "/", require: { permissions: [] } },
    {
      id: "nav.search",
      label: t("navSearch"),
      path: "/search",
      require: {
        permissions: ["inventory.balance.read", "inventory.catalog.read", "reporting.image.read", "search.query"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.inventory",
      label: t("navInventory"),
      path: "/inventory",
      require: { permissions: ["inventory.balance.read"], anyBranch: true, minAmrCount: 1 },
    },
    {
      id: "nav.receiving",
      label: t("navReceiving"),
      path: "/inventory/receiving",
      require: {
        permissions: ["inventory.receipt.create", "inventory.movement.create", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    ...(!isShop
      ? [
          {
            id: "nav.payroll",
            label: t("navPayroll"),
            path: "/payroll",
            require: {
              permissions: ["payroll.run.read"],
              anyBranch: true,
              minAmrCount: 1,
            },
          } satisfies NavNode,
        ]
      : []),
    {
      id: "nav.reports",
      label: t("navReports"),
      path: "/reports",
      require: {
        permissions: ["inventory.balance.read", "payroll.run.read", "reporting.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.imageReports",
      label: t("navImageReports"),
      path: "/reports/images",
      require: {
        permissions: ["reporting.image.read", "reporting.image.create", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
  ];

  const primaryRole = claims?.roles?.[0];

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <h1 className="brand">
          Nexus<span>ERP</span>
        </h1>
        <p className="brand-tag">{t("brandTag")}</p>
        <nav className="nav" aria-label={t("navHome")}>
          {nav.map((node) => {
            const allowed = canAccess(claims, node);
            return (
              <NavLink
                key={node.id}
                to={node.path}
                end={node.path === "/"}
                className={({ isActive }) =>
                  [isActive ? "active" : "", allowed ? "" : "disabled"].filter(Boolean).join(" ")
                }
                onClick={(e) => {
                  if (!allowed) e.preventDefault();
                }}
              >
                {node.label}
              </NavLink>
            );
          })}
        </nav>
      </aside>
      <div className="main">
        <header className="topbar">
          <div>
            <div className="muted">{t("session")}</div>
            <strong>{claims?.sub ? labelUser(claims.sub) : "—"}</strong>{" "}
            <span className="badge">
              {primaryRole ? labelRole(primaryRole, locale) : t("noRole")}
            </span>
          </div>
          <div className="topbar-actions">
            <label className="muted" htmlFor="branch">
              {t("branch")}
            </label>
            <select
              id="branch"
              value={activeBranchId ?? ""}
              onChange={(e) => setActiveBranch(e.target.value)}
            >
              {(claims?.branch_ids ?? []).map((b) => (
                <option key={b} value={b}>
                  {labelBranch(b, locale)}
                </option>
              ))}
            </select>
            <LanguageSwitcher compact />
            <button type="button" className="btn secondary" onClick={logout}>
              {t("logout")}
            </button>
          </div>
        </header>
        <Outlet />
      </div>
    </div>
  );
}
