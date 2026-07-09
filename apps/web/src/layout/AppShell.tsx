import { NavLink, Outlet } from "react-router-dom";
import { useEffect, useState } from "react";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { canAccess, type NavNode } from "../auth/policy";
import { useAuthStore } from "../auth/store";
import { HERO_ROSTER, listenForHeroUnlock } from "../eastereggs/heroes";
import { labelBranch, labelRole, labelUser, useLocaleStore } from "../i18n/locale";

export function AppShell() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const operatorLabel = useAuthStore((s) => s.operatorLabel);
  const setActiveBranch = useAuthStore((s) => s.setActiveBranch);
  const logout = useAuthStore((s) => s.logout);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const isShop = claims?.roles?.includes("store_owner") || claims?.attrs?.profile === "abarrotes";
  const [heroesOpen, setHeroesOpen] = useState(false);

  useEffect(() => listenForHeroUnlock(() => setHeroesOpen(true)), []);

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
    {
      id: "nav.slips",
      label: t("navSlips"),
      path: "/inventory/slips",
      require: {
        permissions: ["inventory.slip.read", "inventory.slip.create", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.transport",
      label: t("navTransport"),
      path: "/inventory/transport",
      require: {
        permissions: ["inventory.transport.read", "inventory.transport.create", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.transfers",
      label: t("navTransfers"),
      path: "/inventory/transfers",
      require: {
        permissions: ["inventory.transfer.read", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.parcels",
      label: t("navParcels"),
      path: "/inventory/parcels",
      require: {
        permissions: ["inventory.parcel.read", "inventory.slip.read", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.storefront",
      label: t("navStorefront"),
      path: "/settings/tienda",
      require: {
        permissions: ["store.storefront.manage", "store.storefront.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.customers",
      label: t("navCustomers"),
      path: "/customers",
      require: {
        permissions: ["customer.read", "customer.card.read", "inventory.balance.read"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.deptManagers",
      label: t("navDeptManagers"),
      path: "/settings/jefes",
      require: {
        permissions: ["store.department.manager.read", "store.department.manager.assign"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.approvals",
      label: t("navApprovals"),
      path: "/approvals",
      require: {
        permissions: ["approval.read", "approval.decide"],
        anyBranch: true,
        minAmrCount: 1,
      },
    },
    {
      id: "nav.mail",
      label: t("navMail"),
      path: "/mail",
      require: {
        permissions: ["mail.read", "mail.send", "inventory.balance.read"],
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
            {operatorLabel ? <span className="badge operator">{operatorLabel}</span> : null}{" "}
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
        {heroesOpen ? (
          <aside className="hero-egg-panel" aria-label={t("heroEggTitle")}>
            <div className="hero-egg-top">
              <h2>{t("heroEggTitle")}</h2>
              <button type="button" className="btn secondary" onClick={() => setHeroesOpen(false)}>
                {t("heroEggClose")}
              </button>
            </div>
            <p className="muted">{t("heroEggLead")}</p>
            <ul className="hero-egg-list">
              {HERO_ROSTER.map((egg) => (
                <li key={egg.call}>
                  <strong>{egg.hero}</strong>
                  <code>{egg.call}</code>
                  <span className="muted">{egg.does}</span>
                  <span className="muted tip">{egg.why}</span>
                </li>
              ))}
            </ul>
          </aside>
        ) : null}
      </div>
    </div>
  );
}
