import { useState } from "react";
import { Navigate } from "react-router-dom";
import { loginWithDevToken, useAuthStore } from "../auth/store";

export function LoginPage() {
  const token = useAuthStore((s) => s.token);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (token) return <Navigate to="/" replace />;

  async function onDevLogin(persona: "analyst" | "approver") {
    setLoading(true);
    setError(null);
    try {
      await loginWithDevToken(persona);
    } catch {
      setError("No se pudo emitir el token de desarrollo. ¿Está el gateway en :8080?");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <h1>NexusERP</h1>
        <p className="muted">
          Acceso seguro con OAuth2/OIDC + MFA. En local usa personas de desarrollo para probar
          RBAC y segregación de funciones en nómina.
        </p>
        <div style={{ display: "grid", gap: "0.75rem" }}>
          <button
            type="button"
            className="btn"
            disabled={loading}
            onClick={() => void onDevLogin("analyst")}
          >
            {loading ? "Autenticando…" : "Entrar como Analista (prepara nómina / inventario)"}
          </button>
          <button
            type="button"
            className="btn secondary"
            disabled={loading}
            onClick={() => void onDevLogin("approver")}
          >
            Entrar como Aprobador (aprueba nómina)
          </button>
        </div>
        <p className="muted" style={{ marginTop: "1rem", fontSize: "0.9rem" }}>
          Producción: Authorization Code + PKCE contra Keycloak (`nexus` realm).
        </p>
        {error ? <p className="error">{error}</p> : null}
      </section>
    </main>
  );
}
