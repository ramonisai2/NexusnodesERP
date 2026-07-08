import { useState } from "react";
import { Navigate } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import { loginWithDevToken, useAuthStore } from "../auth/store";

export function LoginPage() {
  const token = useAuthStore((s) => s.token);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (token) return <Navigate to="/" replace />;

  async function onDevLogin(persona: "analyst" | "approver") {
    setLoading(true);
    setError(null);
    try {
      await loginWithDevToken(persona);
    } catch {
      setError(t("loginError"));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-page">
      <section className="login-card">
        <div className="login-card-top">
          <LanguageSwitcher />
        </div>
        <h1>{t("loginTitle")}</h1>
        <p className="muted">{t("loginSubtitle")}</p>
        <div className="login-actions">
          <button
            type="button"
            className="btn"
            disabled={loading}
            onClick={() => void onDevLogin("analyst")}
          >
            {loading ? t("loginLoading") : t("loginAnalyst")}
          </button>
          <p className="hint">{t("loginAnalystHint")}</p>
          <button
            type="button"
            className="btn secondary"
            disabled={loading}
            onClick={() => void onDevLogin("approver")}
          >
            {t("loginApprover")}
          </button>
          <p className="hint">{t("loginApproverHint")}</p>
        </div>
        <p className="muted secure-note">{t("loginSecureNote")}</p>
        {error ? <p className="error">{error}</p> : null}
        <span className="sr-only" aria-live="polite">
          {locale}
        </span>
      </section>
    </main>
  );
}
