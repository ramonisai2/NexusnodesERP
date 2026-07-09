import { useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { LanguageSwitcher } from "../components/LanguageSwitcher";
import { useLocaleStore } from "../i18n/locale";
import type { MessageKey } from "../i18n/messages";
import { safeCssColor, safeUrl } from "../security/sanitize";

type CatalogItem = {
  sku: string;
  name: string;
  description: string;
  department_label?: string;
  brand?: string;
  currency?: string;
  price?: number | null;
  price_label?: string;
  in_stock?: boolean | null;
  featured?: boolean;
};

type StorefrontView = {
  settings: {
    public_slug: string;
    brand_name: string;
    tagline?: string;
    primary_color: string;
    accent_color: string;
    hero_title: string;
    hero_subtitle?: string;
    hero_image_url?: string;
    cta_label?: string;
    show_prices: boolean;
    show_stock_badge: boolean;
    contact_phone?: string;
    contact_whatsapp?: string;
    contact_email?: string;
    contact_address?: string;
    contact_hours?: string;
    maps_url?: string;
  };
  featured?: CatalogItem[];
  catalog?: CatalogItem[];
};

export function PublicStorefrontPage() {
  const { slug = "" } = useParams();
  const t = useLocaleStore((s) => s.t);

  const view = useQuery({
    queryKey: ["storefront", slug],
    enabled: Boolean(slug),
    queryFn: async () => {
      const res = await fetch(`/api/storefront/public/${encodeURIComponent(slug)}`);
      if (res.status === 404) throw new Error("not_found");
      if (!res.ok) throw new Error("load_failed");
      return (await res.json()) as StorefrontView;
    },
    retry: false,
  });

  if (view.isLoading) {
    return (
      <main className="storefront-public">
        <p className="muted">{t("sfLoading")}</p>
      </main>
    );
  }

  if (view.isError) {
    return (
      <main className="storefront-public">
        <section className="storefront-offline">
          <LanguageSwitcher />
          <h1>{t("sfNotFoundTitle")}</h1>
          <p className="muted">{t("sfNotFoundBody")}</p>
        </section>
      </main>
    );
  }

  const s = view.data!.settings;
  const featured = view.data!.featured ?? [];
  const catalog = view.data!.catalog ?? [];
  const heroImage = safeUrl(s.hero_image_url);
  const mapsUrl = safeUrl(s.maps_url);
  const style = {
    ["--sf-primary" as string]: safeCssColor(s.primary_color, "#1a5c3a"),
    ["--sf-accent" as string]: safeCssColor(s.accent_color, "#c6f2a8"),
  };

  return (
    <main className="storefront-public" style={style}>
      <header className="sf-top">
        <div className="sf-brand">
          <p className="sf-brand-name">{s.brand_name}</p>
          {s.tagline ? <p className="sf-tagline">{s.tagline}</p> : null}
        </div>
        <div className="sf-top-actions">
          <Link className="sf-account-link" to={`/tienda/${encodeURIComponent(slug)}/cuenta`}>
            {t("custAccountLink")}
          </Link>
          <LanguageSwitcher />
        </div>
      </header>

      <section
        className={`sf-hero${heroImage ? " has-image" : ""}`}
        style={
          heroImage
            ? {
                backgroundImage: `linear-gradient(120deg, rgba(10,20,16,0.72), rgba(10,20,16,0.35)), url(${JSON.stringify(heroImage)})`,
              }
            : undefined
        }
      >
        <div className="sf-hero-copy">
          <h1>{s.hero_title || s.brand_name}</h1>
          {s.hero_subtitle ? <p>{s.hero_subtitle}</p> : null}
          {s.cta_label ? (
            <a className="sf-cta" href="#catalogo">
              {s.cta_label}
            </a>
          ) : null}
        </div>
      </section>

      {featured.length > 0 ? (
        <section className="sf-section">
          <h2>{t("sfFeatured")}</h2>
          <div className="sf-grid">
            {featured.map((item) => (
              <ProductTile key={`f-${item.sku}`} item={item} showPrices={s.show_prices} showStock={s.show_stock_badge} t={t} />
            ))}
          </div>
        </section>
      ) : null}

      <section className="sf-section" id="catalogo">
        <h2>{t("sfCatalog")}</h2>
        {catalog.length === 0 ? <p className="muted">{t("sfEmpty")}</p> : null}
        <div className="sf-grid">
          {catalog.map((item) => (
            <ProductTile key={item.sku} item={item} showPrices={s.show_prices} showStock={s.show_stock_badge} t={t} />
          ))}
        </div>
      </section>

      <section className="sf-section sf-contact">
        <h2>{t("sfContact")}</h2>
        <div className="sf-contact-grid">
          {s.contact_phone ? (
            <a href={`tel:${s.contact_phone}`}>{t("sfPhone")}: {s.contact_phone}</a>
          ) : null}
          {s.contact_whatsapp ? (
            <a href={`https://wa.me/${s.contact_whatsapp.replace(/\D/g, "")}`} target="_blank" rel="noreferrer">
              WhatsApp
            </a>
          ) : null}
          {s.contact_email ? <a href={`mailto:${s.contact_email}`}>{s.contact_email}</a> : null}
          {s.contact_address ? <p>{s.contact_address}</p> : null}
          {s.contact_hours ? <p className="muted">{s.contact_hours}</p> : null}
          {mapsUrl ? (
            <a href={mapsUrl} target="_blank" rel="noreferrer">
              {t("sfMaps")}
            </a>
          ) : null}
          {!s.contact_phone && !s.contact_whatsapp && !s.contact_email && !s.contact_address ? (
            <p className="muted">{t("sfNoContact")}</p>
          ) : null}
        </div>
      </section>

      <footer className="sf-foot">
        <span>{s.brand_name}</span>
        <span className="muted">NexusERP</span>
      </footer>
    </main>
  );
}

function ProductTile({
  item,
  showPrices,
  showStock,
  t,
}: {
  item: CatalogItem;
  showPrices: boolean;
  showStock: boolean;
  t: (k: MessageKey) => string;
}) {
  return (
    <article className={`sf-product${item.featured ? " featured" : ""}`}>
      <div className="sf-product-top">
        {item.department_label ? <span className="sf-dept">{item.department_label}</span> : null}
        {showStock && item.in_stock != null ? (
          <span className={`sf-stock${item.in_stock ? " ok" : ""}`}>
            {item.in_stock ? t("sfInStock") : t("sfOutStock")}
          </span>
        ) : null}
      </div>
      <h3>{item.name}</h3>
      {item.description && item.description !== item.name ? <p className="muted">{item.description}</p> : null}
      <div className="sf-product-meta">
        <span className="muted">{item.sku}</span>
        {showPrices && item.price != null ? (
          <strong>
            {item.currency || "MXN"} {Number(item.price).toFixed(2)}
          </strong>
        ) : null}
      </div>
    </article>
  );
}
