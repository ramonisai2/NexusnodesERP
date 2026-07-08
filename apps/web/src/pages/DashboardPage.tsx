import { useAuthStore } from "../auth/store";

export function DashboardPage() {
  const claims = useAuthStore((s) => s.claims);

  return (
    <section className="panel">
      <h1>Escritorio</h1>
      <p className="muted">
        Nodos de UI y APIs se condicionan por permisos JWT, sucursal activa y nivel MFA.
      </p>
      <div className="grid stats" style={{ marginTop: "1.25rem" }}>
        <div className="stat">
          <span className="muted">Organización</span>
          <strong>{claims?.org_id}</strong>
        </div>
        <div className="stat">
          <span className="muted">Permisos</span>
          <strong>{claims?.permissions.length ?? 0}</strong>
        </div>
        <div className="stat">
          <span className="muted">Sucursales</span>
          <strong>{claims?.branch_ids.length ?? 0}</strong>
        </div>
      </div>
      <h2>Permisos efectivos</h2>
      <ul>
        {(claims?.permissions ?? []).map((p) => (
          <li key={p}>
            <code>{p}</code>
          </li>
        ))}
      </ul>
    </section>
  );
}
