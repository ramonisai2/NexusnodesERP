import { useLocaleStore } from "../i18n/locale";
import type { Locale } from "../i18n/messages";

export function LanguageSwitcher({ compact = false }: { compact?: boolean }) {
  const locale = useLocaleStore((s) => s.locale);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const t = useLocaleStore((s) => s.t);

  return (
    <label className="lang-switch" htmlFor="locale">
      {!compact ? <span className="muted">{t("language")}</span> : null}
      <select
        id="locale"
        value={locale}
        aria-label={t("language")}
        onChange={(e) => setLocale(e.target.value as Locale)}
      >
        <option value="es">Español</option>
        <option value="en">English</option>
      </select>
    </label>
  );
}
