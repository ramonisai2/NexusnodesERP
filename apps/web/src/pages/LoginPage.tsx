import { useMemo, useState } from "react";
import { Link, Navigate } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import {
  DEV_PERSONAS,
  loginWithDevToken,
  useAuthStore,
  type DevPersona,
} from "../auth/store";

export function LoginPage() {
  const token = useAuthStore((s) => s.token);
  const t = useLocaleStore((s) => s.t);
  const locale = useLocaleStore((s) => s.locale);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [operatorLabel, setOperatorLabel] = useState("");
  const [stationId, setStationId] = useState("");
  const [persona, setPersona] = useState<DevPersona>("warehouse");

  const selected = useMemo(
    () => DEV_PERSONAS.find((p) => p.id === persona) ?? DEV_PERSONAS[0],
    [persona],
  );

  if (token) return <Navigate to="/" replace />;

  async function onDevLogin(p: DevPersona) {
    setLoading(true);
    setError(null);
    try {
      const op = operatorLabel.trim();
      await loginWithDevToken(p, {
        operatorLabel: op || undefined,
        stationId: stationId.trim() || undefined,
      });
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

        <div className="login-operator">
          <label htmlFor="operator">
            {t("loginOperatorLabel")}
            <input
              id="operator"
              type="text"
              value={operatorLabel}
              onChange={(e) => setOperatorLabel(e.target.value)}
              placeholder={t("loginOperatorPh")}
              autoComplete="nickname"
            />
          </label>
          <label htmlFor="station">
            {t("loginStationLabel")}
            <input
              id="station"
              type="text"
              value={stationId}
              onChange={(e) => setStationId(e.target.value)}
              placeholder={t("loginStationPh")}
              autoComplete="off"
            />
          </label>
          <p className="hint">{t("loginOperatorHint")}</p>
        </div>

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
            <label htmlFor="persona">
              {t("loginPersonaLabel")}
              <select
                id="persona"
                value={persona}
                onChange={(e) => setPersona(e.target.value as DevPersona)}
                disabled={loading}
              >
                {DEV_PERSONAS.map((p) => (
                  <option key={p.id} value={p.id}>
                    {t(p.labelKey)}
                  </option>
                ))}
              </select>
            </label>
            <p className="hint">{t(selected.hintKey)}</p>
            <button
              type="button"
              className="btn secondary"
              disabled={loading}
              onClick={() => void onDevLogin(persona)}
            >
              {loading ? t("loginLoading") : t("loginPersonaCta")}
            </button>
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
