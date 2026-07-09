package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ramonisai2/NexusnodesERP/apps/inventory/internal/domain"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func (s *Postgres) GetStorefrontSettings(ctx context.Context, orgRef, branchCode string) (domain.StorefrontSettings, error) {
	var out domain.StorefrontSettings
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		orgID, err := resolveOrgID(ctx, tx, orgRef)
		if err != nil {
			return err
		}
		branchID, err := resolveBranchID(ctx, tx, orgID, branchCode)
		if err != nil {
			return err
		}
		err = scanStorefront(ctx, tx, `
SELECT s.id::text, s.org_id::text, b.code, s.public_slug, s.published,
       s.brand_name, s.tagline, s.primary_color, s.accent_color,
       s.hero_title, s.hero_subtitle, s.hero_image_url, s.cta_label, s.cta_url,
       s.show_prices, s.show_stock_badge, s.in_stock_only, s.featured_skus,
       s.contact_phone, s.contact_whatsapp, s.contact_email, s.contact_address,
       s.contact_hours, s.maps_url, s.updated_at
FROM branch_storefront_settings s
JOIN branches b ON b.id = s.branch_id
WHERE s.org_id = $1::uuid AND s.branch_id = $2::uuid`, orgID, branchID, &out)
		if errors.Is(err, pgx.ErrNoRows) {
			var orgName, branchName string
			_ = tx.QueryRow(ctx, `SELECT name FROM organizations WHERE id = $1::uuid`, orgID).Scan(&orgName)
			_ = tx.QueryRow(ctx, `SELECT name FROM branches WHERE id = $1::uuid`, branchID).Scan(&branchName)
			out = domain.StorefrontSettings{
				OrgID:          orgID,
				BranchID:       branchCode,
				PublicSlug:     slugify(orgName + "-" + branchCode),
				Published:      false,
				BrandName:      orgName,
				Tagline:        branchName,
				PrimaryColor:   "#1a5c3a",
				AccentColor:    "#c6f2a8",
				HeroTitle:      orgName,
				HeroSubtitle:   "Catálogo de productos disponibles",
				CTALabel:       "Ver productos",
				ShowPrices:     true,
				ShowStockBadge: true,
				InStockOnly:    true,
				FeaturedSKUs:   []string{},
			}
			out.PublicURL = publicStoreURL(out.PublicSlug)
			return nil
		}
		if err != nil {
			return err
		}
		if out.FeaturedSKUs == nil {
			out.FeaturedSKUs = []string{}
		}
		out.PublicURL = publicStoreURL(out.PublicSlug)
		return nil
	})
	return out, err
}

func (s *Postgres) UpsertStorefrontSettings(ctx context.Context, req domain.UpsertStorefrontRequest) (domain.StorefrontSettings, error) {
	slug := normalizeSlug(req.PublicSlug)
	if slug == "" {
		return domain.StorefrontSettings{}, errors.New("public_slug required")
	}
	if strings.TrimSpace(req.BrandName) == "" {
		return domain.StorefrontSettings{}, errors.New("brand_name required")
	}
	if strings.TrimSpace(req.BranchID) == "" {
		return domain.StorefrontSettings{}, errors.New("branch_id required")
	}
	// Injection / XSS prevention on user-controlled marketing fields.
	for _, pair := range [][2]string{
		{"brand_name", req.BrandName},
		{"tagline", req.Tagline},
		{"hero_title", req.HeroTitle},
		{"hero_subtitle", req.HeroSubtitle},
		{"cta_label", req.CTALabel},
		{"contact_address", req.ContactAddress},
	} {
		if msg := secure.RejectIfInjection(pair[0], pair[1]); msg != "" {
			return domain.StorefrontSettings{}, errors.New(msg)
		}
	}
	req.BrandName = secure.PlainTextMax(req.BrandName, 120)
	req.Tagline = secure.PlainTextMax(req.Tagline, 200)
	req.HeroTitle = secure.PlainTextMax(req.HeroTitle, 160)
	req.HeroSubtitle = secure.PlainTextMax(req.HeroSubtitle, 280)
	req.CTALabel = secure.PlainTextMax(req.CTALabel, 80)
	req.ContactPhone = secure.PlainTextMax(req.ContactPhone, 40)
	req.ContactWhatsApp = secure.PlainTextMax(req.ContactWhatsApp, 40)
	req.ContactEmail = secure.PlainTextMax(req.ContactEmail, 120)
	req.ContactAddress = secure.PlainTextMax(req.ContactAddress, 240)
	req.ContactHours = secure.PlainTextMax(req.ContactHours, 120)
	req.HeroImageURL = secure.SafeURL(req.HeroImageURL)
	req.CTAURL = secure.SafeURL(req.CTAURL)
	req.MapsURL = secure.SafeURL(req.MapsURL)
	primary := secure.SafeCSSColor(req.PrimaryColor, "#1a5c3a")
	accent := secure.SafeCSSColor(req.AccentColor, "#c6f2a8")
	featured := uniqueStrings(req.FeaturedSKUs)
	if featured == nil {
		featured = []string{}
	}

	tx, orgID, err := db.BeginOrgTx(ctx, s.pool, func(tx pgx.Tx) (string, error) {
		return resolveOrgID(ctx, tx, req.OrgID)
	})
	if err != nil {
		return domain.StorefrontSettings{}, err
	}
	defer tx.Rollback(ctx)

	branchID, err := resolveBranchID(ctx, tx, orgID, req.BranchID)
	if err != nil {
		return domain.StorefrontSettings{}, err
	}

	var conflict string
	err = tx.QueryRow(ctx, `
SELECT branch_id::text FROM branch_storefront_settings
WHERE public_slug = $1 AND branch_id <> $2::uuid`, slug, branchID).Scan(&conflict)
	if err == nil {
		return domain.StorefrontSettings{}, errors.New("public_slug already taken")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.StorefrontSettings{}, err
	}

	now := time.Now().UTC()
	id := uuid.New()
	published := req.Published
	if published {
		var netMode string
		_ = tx.QueryRow(ctx, `
SELECT COALESCE(network_mode, 'intranet') FROM organizations WHERE id = $1::uuid`, orgID).Scan(&netMode)
		if strings.ToLower(strings.TrimSpace(netMode)) != "internet" {
			return domain.StorefrontSettings{}, errors.New("network_mode_intranet: publish requires internet mode")
		}
	}
	_, err = tx.Exec(ctx, `
INSERT INTO branch_storefront_settings (
  id, org_id, branch_id, public_slug, published, brand_name, tagline,
  primary_color, accent_color, hero_title, hero_subtitle, hero_image_url,
  cta_label, cta_url, show_prices, show_stock_badge, in_stock_only, featured_skus,
  contact_phone, contact_whatsapp, contact_email, contact_address, contact_hours,
  maps_url, updated_by, created_at, updated_at
) VALUES (
  $1, $2::uuid, $3::uuid, $4, $5, $6, $7,
  $8, $9, $10, $11, $12,
  $13, $14, $15, $16, $17, $18,
  $19, $20, $21, $22, $23,
  $24, $25, $26, $26
)
ON CONFLICT (branch_id) DO UPDATE SET
  public_slug = EXCLUDED.public_slug,
  published = EXCLUDED.published,
  brand_name = EXCLUDED.brand_name,
  tagline = EXCLUDED.tagline,
  primary_color = EXCLUDED.primary_color,
  accent_color = EXCLUDED.accent_color,
  hero_title = EXCLUDED.hero_title,
  hero_subtitle = EXCLUDED.hero_subtitle,
  hero_image_url = EXCLUDED.hero_image_url,
  cta_label = EXCLUDED.cta_label,
  cta_url = EXCLUDED.cta_url,
  show_prices = EXCLUDED.show_prices,
  show_stock_badge = EXCLUDED.show_stock_badge,
  in_stock_only = EXCLUDED.in_stock_only,
  featured_skus = EXCLUDED.featured_skus,
  contact_phone = EXCLUDED.contact_phone,
  contact_whatsapp = EXCLUDED.contact_whatsapp,
  contact_email = EXCLUDED.contact_email,
  contact_address = EXCLUDED.contact_address,
  contact_hours = EXCLUDED.contact_hours,
  maps_url = EXCLUDED.maps_url,
  updated_by = EXCLUDED.updated_by,
  updated_at = EXCLUDED.updated_at`,
		id, orgID, branchID, slug, published, strings.TrimSpace(req.BrandName), strings.TrimSpace(req.Tagline),
		primary, accent, strings.TrimSpace(req.HeroTitle), strings.TrimSpace(req.HeroSubtitle), strings.TrimSpace(req.HeroImageURL),
		strings.TrimSpace(req.CTALabel), strings.TrimSpace(req.CTAURL), req.ShowPrices, req.ShowStockBadge, req.InStockOnly, featured,
		strings.TrimSpace(req.ContactPhone), strings.TrimSpace(req.ContactWhatsApp), strings.TrimSpace(req.ContactEmail),
		strings.TrimSpace(req.ContactAddress), strings.TrimSpace(req.ContactHours), strings.TrimSpace(req.MapsURL),
		req.UpdatedBy, now)
	if err != nil {
		return domain.StorefrontSettings{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.StorefrontSettings{}, err
	}
	return s.GetStorefrontSettings(ctx, req.OrgID, req.BranchID)
}

func (s *Postgres) GetPublicStorefront(ctx context.Context, slug string) (domain.StorefrontPublicView, error) {
	slug = normalizeSlug(slug)
	if slug == "" {
		return domain.StorefrontPublicView{}, domain.ErrNotFound
	}
	var view domain.StorefrontPublicView
	err := db.WithOrgTx(ctx, s.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		var settings domain.StorefrontSettings
		var branchUUID string
		err := tx.QueryRow(ctx, `
SELECT s.id::text, s.org_id::text, b.code, s.public_slug, s.published,
       s.brand_name, s.tagline, s.primary_color, s.accent_color,
       s.hero_title, s.hero_subtitle, s.hero_image_url, s.cta_label, s.cta_url,
       s.show_prices, s.show_stock_badge, s.in_stock_only, s.featured_skus,
       s.contact_phone, s.contact_whatsapp, s.contact_email, s.contact_address,
       s.contact_hours, s.maps_url, s.updated_at, s.branch_id::text
FROM branch_storefront_settings s
JOIN branches b ON b.id = s.branch_id
JOIN organizations o ON o.id = s.org_id
WHERE s.public_slug = $1 AND s.published = TRUE
  AND COALESCE(o.network_mode, 'intranet') = 'internet'`, slug).Scan(
			&settings.ID, &settings.OrgID, &settings.BranchID, &settings.PublicSlug, &settings.Published,
			&settings.BrandName, &settings.Tagline, &settings.PrimaryColor, &settings.AccentColor,
			&settings.HeroTitle, &settings.HeroSubtitle, &settings.HeroImageURL, &settings.CTALabel, &settings.CTAURL,
			&settings.ShowPrices, &settings.ShowStockBadge, &settings.InStockOnly, &settings.FeaturedSKUs,
			&settings.ContactPhone, &settings.ContactWhatsApp, &settings.ContactEmail, &settings.ContactAddress,
			&settings.ContactHours, &settings.MapsURL, &settings.UpdatedAt, &branchUUID,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		if settings.FeaturedSKUs == nil {
			settings.FeaturedSKUs = []string{}
		}
		settings.PublicURL = publicStoreURL(settings.PublicSlug)
		view.Settings = settings

		items, err := loadStorefrontCatalog(ctx, tx, branchUUID, settings)
		if err != nil {
			return err
		}
		featuredSet := map[string]struct{}{}
		for _, sku := range settings.FeaturedSKUs {
			featuredSet[strings.ToUpper(sku)] = struct{}{}
		}
		var featured, catalog []domain.StorefrontCatalogItem
		for _, it := range items {
			if _, ok := featuredSet[strings.ToUpper(it.SKU)]; ok {
				it.Featured = true
				featured = append(featured, it)
			}
			catalog = append(catalog, it)
		}
		ordered := make([]domain.StorefrontCatalogItem, 0, len(featured))
		bySKU := map[string]domain.StorefrontCatalogItem{}
		for _, f := range featured {
			bySKU[strings.ToUpper(f.SKU)] = f
		}
		for _, sku := range settings.FeaturedSKUs {
			if f, ok := bySKU[strings.ToUpper(sku)]; ok {
				ordered = append(ordered, f)
			}
		}
		view.Featured = ordered
		view.Catalog = catalog
		return nil
	})
	return view, err
}

func loadStorefrontCatalog(ctx context.Context, tx pgx.Tx, branchUUID string, settings domain.StorefrontSettings) ([]domain.StorefrontCatalogItem, error) {
	rows, err := tx.Query(ctx, `
SELECT ps.sku,
       COALESCE(NULLIF(l.public_description,''), p.name, ps.sku),
       COALESCE(l.public_description, ''),
       COALESCE(l.department_label, ''),
       COALESCE(NULLIF(l.brand_label, ''), ps.brand, ''),
       l.currency,
       l.common_price::float8,
       l.special_price::float8,
       l.final_price::float8,
       l.price_mode,
       COALESCE((
         SELECT SUM(sb.on_hand)::float8
         FROM stock_balances sb
         JOIN warehouses w ON w.id = sb.warehouse_id
         WHERE sb.sku_id = ps.id AND w.branch_id = l.branch_id
       ), 0)
FROM store_sku_labels l
JOIN product_skus ps ON ps.id = l.sku_id
JOIN products p ON p.id = ps.product_id
WHERE l.branch_id = $1::uuid AND l.active = TRUE
ORDER BY ps.sku`, branchUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.StorefrontCatalogItem
	for rows.Next() {
		var sku, name, desc, dept, brand, currency, mode string
		var common, special, final *float64
		var onHand float64
		if err := rows.Scan(&sku, &name, &desc, &dept, &brand, &currency, &common, &special, &final, &mode, &onHand); err != nil {
			return nil, err
		}
		if settings.InStockOnly && onHand <= 0 {
			continue
		}
		item := domain.StorefrontCatalogItem{
			SKU:             sku,
			Name:            name,
			Description:     desc,
			DepartmentLabel: dept,
			Brand:           brand,
		}
		if settings.ShowPrices {
			price := resolveStorefrontPrice(mode, common, special, final)
			item.Currency = currency
			item.Price = price
			item.PriceLabel = storefrontPriceModeLabel(mode)
		}
		if settings.ShowStockBadge {
			in := onHand > 0
			item.InStock = &in
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func resolveStorefrontPrice(mode string, common, special, final *float64) *float64 {
	switch strings.ToUpper(mode) {
	case "SPECIAL":
		if special != nil {
			return special
		}
	case "FINAL":
		if final != nil {
			return final
		}
	}
	return common
}

func storefrontPriceModeLabel(mode string) string {
	switch strings.ToUpper(mode) {
	case "SPECIAL":
		return "Especial"
	case "FINAL":
		return "Final"
	default:
		return "Común"
	}
}

func scanStorefront(ctx context.Context, tx pgx.Tx, q string, orgID, branchID string, out *domain.StorefrontSettings) error {
	return tx.QueryRow(ctx, q, orgID, branchID).Scan(
		&out.ID, &out.OrgID, &out.BranchID, &out.PublicSlug, &out.Published,
		&out.BrandName, &out.Tagline, &out.PrimaryColor, &out.AccentColor,
		&out.HeroTitle, &out.HeroSubtitle, &out.HeroImageURL, &out.CTALabel, &out.CTAURL,
		&out.ShowPrices, &out.ShowStockBadge, &out.InStockOnly, &out.FeaturedSKUs,
		&out.ContactPhone, &out.ContactWhatsApp, &out.ContactEmail, &out.ContactAddress,
		&out.ContactHours, &out.MapsURL, &out.UpdatedAt,
	)
}

func normalizeSlug(s string) string {
	s = slugify(s)
	if len(s) > 64 {
		s = s[:64]
	}
	return strings.Trim(s, "-")
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	repl := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
		" ", "-", "_", "-",
	)
	s = repl.Replace(s)
	s = slugRe.ReplaceAllString(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func publicStoreURL(slug string) string {
	base := strings.TrimRight(os.Getenv("PUBLIC_WEB_BASE"), "/")
	if base == "" {
		base = "http://localhost:5173"
	}
	return fmt.Sprintf("%s/tienda/%s", base, slug)
}
