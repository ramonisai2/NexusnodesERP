import { NavLink, Outlet } from "react-router-dom";
import { NAV_NODES, canAccess } from "../auth/policy";
import { useAuthStore } from "../auth/store";

export function AppShell() {
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const setActiveBranch = useAuthStore((s) => s.setActiveBranch);
  const logout = useAuthStore((s) => s.logout);

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <h1 className="brand">
          Nexus<span>ERP</span>
        </h1>
        <p className="brand-tag">Inventarios · Nóminas</p>
        <nav className="nav" aria-label="Principal">
          {NAV_NODES.map((node) => {
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
            <div className="muted">Sesión</div>
            <strong>{claims?.sub ?? "—"}</strong>{" "}
            <span className="badge">{claims?.roles?.[0] ?? "sin rol"}</span>
          </div>
          <div style={{ display: "flex", gap: "0.75rem", alignItems: "center" }}>
            <label className="muted" htmlFor="branch">
              Sucursal
            </label>
            <select
              id="branch"
              value={activeBranchId ?? ""}
              onChange={(e) => setActiveBranch(e.target.value)}
            >
              {(claims?.branch_ids ?? []).map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
            <button type="button" className="btn secondary" onClick={logout}>
              Salir
            </button>
          </div>
        </header>
        <Outlet />
      </div>
    </div>
  );
}
