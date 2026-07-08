import { useState } from "react";
import { Link, Navigate } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import { loginWithDevToken, useAuthStore } from "../auth/store";

type Persona = "owner" | "analyst" | "approver" | "wh_manager" | "regional";

export function LoginPage() {
  const token = useAuthStore((s) => s.token);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);

  if (token) return <Navigate to="/" replace />;

  async function onDevLogin(persona: Persona) {
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
        <p className="muted">{t("loginSubtitleShop")}</p>

        <div className="login-actions">
          <Link className="btn" to="/setup">
            {t("loginSetupCta")}
          </Link>
          <p className="hint">{t("loginSetupHint")}</p>

          <button
            type="button"
            className="btn secondary"
            disabled={loading}
            onClick={() => void onDevLogin("owner")}
          >
            {loading ? t("loginLoading") : t("loginOwner")}
          </button>
          <p className="hint">{t("loginOwnerHint")}</p>
        </div>

        <button
          type="button"
          className="linkish"
          onClick={() => setShowAdvanced((v) => !v)}
        >
          {showAdvanced ? t("loginHideAdvanced") : t("loginShowAdvanced")}
        </button>

        {showAdvanced ? (
          <div className="login-actions advanced">
            <button type="button" className="btn secondary" disabled={loading} onClick={() => void onDevLogin("analyst")}>
              {t("loginAnalyst")}
            </button>
            <p className="hint">{t("loginAnalystHint")}</p>
            <button type="button" className="btn secondary" disabled={loading} onClick={() => void onDevLogin("approver")}>
              {t("loginApprover")}
            </button>
            <p className="hint">{t("loginApproverHint")}</p>
            <button type="button" className="btn secondary" disabled={loading} onClick={() => void onDevLogin("wh_manager")}>
              {t("loginWhManager")}
            </button>
            <p className="hint">{t("loginWhManagerHint")}</p>
            <button type="button" className="btn secondary" disabled={loading} onClick={() => void onDevLogin("regional")}>
              {t("loginRegional")}
            </button>
            <p className="hint">{t("loginRegionalHint")}</p>
          </div>
        ) : null}

        <p className="muted secure-note">{t("loginSecureNote")}</p>
        {error ? <p className="error">{error}</p> : null}
        <span className="sr-only" aria-live="polite">
          {locale}
        </span>
      </section>
    </main>
  );
}
