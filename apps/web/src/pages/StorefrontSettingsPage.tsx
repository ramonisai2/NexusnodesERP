import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { hasPermission } from "../auth/policy";
import { PolicyGuard } from "../auth/PolicyGuard";
import { apiFetch, useAuthStore } from "../auth/store";
import { useLocaleStore } from "../i18n/locale";
import { rejectIfInjection, safeUrl } from "../security/sanitize";

type StorefrontSettings = {
  branch_id: string;
  public_slug: string;
  published: boolean;
  brand_name: string;
  tagline?: string;
  primary_color: string;
  accent_color: string;
  hero_title: string;
  hero_subtitle?: string;
  hero_image_url?: string;
  cta_label?: string;
  cta_url?: string;
  show_prices: boolean;
  show_stock_badge: boolean;
  in_stock_only: boolean;
  featured_skus?: string[];
  contact_phone?: string;
  contact_whatsapp?: string;
  contact_email?: string;
  contact_address?: string;
  contact_hours?: string;
  maps_url?: string;
  public_url?: string;
};

const node = {
  id: "nav.storefront",
  label: "Tienda en línea",
  path: "/settings/tienda",
  require: {
    permissions: ["store.storefront.manage", "store.storefront.read"],
    anyBranch: true,
    minAmrCount: 1,
  },
};

export function StorefrontSettingsPage() {
  const t = useLocaleStore((s) => s.t);
  return (
    <PolicyGuard
      node={node}
      fallback={
        <section className="panel">
          <h1>{t("sfAdminTitle")}</h1>
          <p className="error">{t("sfAdminForbidden")}</p>
        </section>
      }
    >
      <StorefrontSettingsPanel />
    </PolicyGuard>
  );
}

function StorefrontSettingsPanel() {
  const t = useLocaleStore((s) => s.t);
  const claims = useAuthStore((s) => s.claims);
  const activeBranchId = useAuthStore((s) => s.activeBranchId);
  const branchId = activeBranchId || claims?.branch_ids?.[0] || "";
  const qc = useQueryClient();
  const canManage =
    hasPermission(claims, "store.storefront.manage") || claims?.roles?.includes("store_owner");

  const [form, setForm] = useState<StorefrontSettings | null>(null);
  const [featuredText, setFeaturedText] = useState("");
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  const settings = useQuery({
    queryKey: ["storefront-settings", branchId],
    enabled: !!branchId,
    queryFn: async () => {
      const res = await apiFetch(`/storefront/settings?branch_id=${encodeURIComponent(branchId)}`);
      if (!res.ok) throw new Error("load_failed");
      return (await res.json()) as StorefrontSettings;
    },
  });

  useEffect(() => {
    if (settings.data) {
      setForm(settings.data);
      setFeaturedText((settings.data.featured_skus ?? []).join(", "));
    }
  }, [settings.data]);

  const save = useMutation({
    mutationFn: async () => {
      if (!form) throw new Error("no_form");
      const injection = rejectIfInjection(
        form.brand_name,
        form.tagline,
        form.hero_title,
        form.hero_subtitle,
        form.cta_label,
        form.hero_image_url,
        form.cta_url,
        form.contact_phone,
        form.contact_whatsapp,
        form.contact_email,
        form.contact_address,
        form.contact_hours,
        form.maps_url,
      );
      if (injection) throw new Error(injection);
      const payload = {
        ...form,
        branch_id: branchId,
        hero_image_url: safeUrl(form.hero_image_url) || "",
        cta_url: safeUrl(form.cta_url) || "",
        maps_url: safeUrl(form.maps_url) || "",
        featured_skus: featuredText
          .split(/[,\n]/)
          .map((s) => s.trim())
          .filter(Boolean),
      };
      const res = await apiFetch("/storefront/settings", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      if (!res.ok) throw new Error(await res.text());
      return (await res.json()) as StorefrontSettings;
    },
    onSuccess: (data) => {
      setForm(data);
      setFeaturedText((data.featured_skus ?? []).join(", "));
      setSaved(true);
      setError("");
      void qc.invalidateQueries({ queryKey: ["storefront-settings"] });
    },
    onError: (err: Error) => setError(err.message || t("sfAdminSaveError")),
  });

  if (settings.isLoading || !form) {
    return (
      <section className="panel">
        <p className="muted">{t("sfAdminLoading")}</p>
      </section>
    );
  }

  const set = <K extends keyof StorefrontSettings>(key: K, value: StorefrontSettings[K]) =>
    setForm((prev) => (prev ? { ...prev, [key]: value } : prev));

  return (
    <section className="panel storefront-admin">
      <header className="transfer-header">
        <div>
          <h1>{t("sfAdminTitle")}</h1>
          <p className="muted">{t("sfAdminSubtitle")}</p>
        </div>
        {form.public_url ? (
          <Link className="btn" to={`/tienda/${form.public_slug}`} target="_blank">
            {t("sfAdminPreview")}
          </Link>
        ) : null}
      </header>

      <form
        className="sf-admin-form"
        onSubmit={(e) => {
          e.preventDefault();
          if (!canManage) return;
          setSaved(false);
          save.mutate();
        }}
      >
        <label className="mail-field">
          <span>{t("sfAdminSlug")}</span>
          <input value={form.public_slug} onChange={(e) => set("public_slug", e.target.value)} required />
          <span className="muted">/tienda/{form.public_slug || "…"}</span>
        </label>

        <label className="sf-check">
          <input
            type="checkbox"
            checked={form.published}
            onChange={(e) => set("published", e.target.checked)}
            disabled={!canManage}
          />
          <span>{t("sfAdminPublished")}</span>
        </label>

        <div className="transfer-wh-row">
          <label className="mail-field">
            <span>{t("sfAdminBrand")}</span>
            <input value={form.brand_name} onChange={(e) => set("brand_name", e.target.value)} required />
          </label>
          <label className="mail-field">
            <span>{t("sfAdminTagline")}</span>
            <input value={form.tagline || ""} onChange={(e) => set("tagline", e.target.value)} />
          </label>
        </div>

        <div className="transfer-wh-row">
          <label className="mail-field">
            <span>{t("sfAdminPrimary")}</span>
            <input type="color" value={form.primary_color || "#1a5c3a"} onChange={(e) => set("primary_color", e.target.value)} />
          </label>
          <label className="mail-field">
            <span>{t("sfAdminAccent")}</span>
            <input type="color" value={form.accent_color || "#c6f2a8"} onChange={(e) => set("accent_color", e.target.value)} />
          </label>
        </div>

        <label className="mail-field">
          <span>{t("sfAdminHeroTitle")}</span>
          <input value={form.hero_title} onChange={(e) => set("hero_title", e.target.value)} />
        </label>
        <label className="mail-field">
          <span>{t("sfAdminHeroSubtitle")}</span>
          <textarea rows={2} value={form.hero_subtitle || ""} onChange={(e) => set("hero_subtitle", e.target.value)} />
        </label>
        <label className="mail-field">
          <span>{t("sfAdminHeroImage")}</span>
          <input value={form.hero_image_url || ""} onChange={(e) => set("hero_image_url", e.target.value)} placeholder="https://…" />
        </label>
        <label className="mail-field">
          <span>{t("sfAdminCta")}</span>
          <input value={form.cta_label || ""} onChange={(e) => set("cta_label", e.target.value)} />
        </label>

        <div className="sf-toggles">
          <label className="sf-check">
            <input type="checkbox" checked={form.show_prices} onChange={(e) => set("show_prices", e.target.checked)} />
            <span>{t("sfAdminShowPrices")}</span>
          </label>
          <label className="sf-check">
            <input type="checkbox" checked={form.show_stock_badge} onChange={(e) => set("show_stock_badge", e.target.checked)} />
            <span>{t("sfAdminShowStock")}</span>
          </label>
          <label className="sf-check">
            <input type="checkbox" checked={form.in_stock_only} onChange={(e) => set("in_stock_only", e.target.checked)} />
            <span>{t("sfAdminInStockOnly")}</span>
          </label>
        </div>

        <label className="mail-field">
          <span>{t("sfAdminFeatured")}</span>
          <input value={featuredText} onChange={(e) => setFeaturedText(e.target.value)} placeholder="SKU1, SKU2" />
        </label>

        <h2>{t("sfContact")}</h2>
        <div className="transfer-wh-row">
          <label className="mail-field">
            <span>{t("sfPhone")}</span>
            <input value={form.contact_phone || ""} onChange={(e) => set("contact_phone", e.target.value)} />
          </label>
          <label className="mail-field">
            <span>WhatsApp</span>
            <input value={form.contact_whatsapp || ""} onChange={(e) => set("contact_whatsapp", e.target.value)} />
          </label>
        </div>
        <label className="mail-field">
          <span>Email</span>
          <input value={form.contact_email || ""} onChange={(e) => set("contact_email", e.target.value)} />
        </label>
        <label className="mail-field">
          <span>{t("sfAdminAddress")}</span>
          <input value={form.contact_address || ""} onChange={(e) => set("contact_address", e.target.value)} />
        </label>
        <label className="mail-field">
          <span>{t("sfAdminHours")}</span>
          <input value={form.contact_hours || ""} onChange={(e) => set("contact_hours", e.target.value)} />
        </label>
        <label className="mail-field">
          <span>{t("sfMaps")}</span>
          <input value={form.maps_url || ""} onChange={(e) => set("maps_url", e.target.value)} />
        </label>

        {error ? <p className="error">{error}</p> : null}
        {saved ? <p className="muted">{t("sfAdminSaved")}</p> : null}

        {canManage ? (
          <button type="submit" className="btn primary" disabled={save.isPending}>
            {save.isPending ? t("sfAdminSaving") : t("sfAdminSave")}
          </button>
        ) : (
          <p className="muted">{t("sfAdminReadOnly")}</p>
        )}
      </form>
    </section>
  );
}
