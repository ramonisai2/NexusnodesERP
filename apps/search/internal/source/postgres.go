package source

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ramonisai2/NexusnodesERP/apps/search/internal/engine"
	"github.com/ramonisai2/NexusnodesERP/packages/go/db"
)

type Postgres struct {
	pool *pgxpool.Pool
}

func NewPostgres(pool *pgxpool.Pool) *Postgres {
	return &Postgres{pool: pool}
}

func (p *Postgres) LoadAll(ctx context.Context) ([]engine.Document, error) {
	var docs []engine.Document
	err := db.WithOrgTx(ctx, p.pool, "", func(tx pgx.Tx) error {
		if err := db.SetRLSBypass(ctx, tx, true); err != nil {
			return err
		}
		products, err := loadProducts(ctx, tx)
		if err != nil {
			return fmt.Errorf("products: %w", err)
		}
		labels, err := loadLabels(ctx, tx)
		if err != nil {
			return fmt.Errorf("labels: %w", err)
		}
		photos, err := loadPhotos(ctx, tx)
		if err != nil {
			return fmt.Errorf("photos: %w", err)
		}
		docs = append(docs, products...)
		docs = append(docs, labels...)
		docs = append(docs, photos...)
		return nil
	})
	return docs, err
}

func loadProducts(ctx context.Context, tx pgx.Tx) ([]engine.Document, error) {
	rows, err := tx.Query(ctx, `
SELECT
  o.id::text AS org_id,
  o.code AS org_code,
  p.id::text AS product_id,
  COALESCE(p.name, '') AS name,
  COALESCE(p.sku_base, '') AS sku_base,
  COALESCE(ps.sku, '') AS sku,
  COALESCE(ps.barcode, '') AS barcode,
  COALESCE(ps.material_code, '') AS material,
  COALESCE(ps.brand, '') AS brand,
  COALESCE(string_agg(DISTINCT sd.code, ' '), '') AS depts
FROM products p
JOIN organizations o ON o.id = p.org_id
LEFT JOIN product_skus ps ON ps.product_id = p.id
LEFT JOIN product_placements pp ON pp.product_id = p.id AND pp.active
LEFT JOIN store_departments sd ON sd.id = pp.department_id AND sd.active
GROUP BY o.id, o.code, p.id, p.name, p.sku_base, ps.sku, ps.barcode, ps.material_code, ps.brand
ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []engine.Document
	for rows.Next() {
		var orgID, orgCode, productID, name, skuBase, sku, barcode, material, brand, depts string
		if err := rows.Scan(&orgID, &orgCode, &productID, &name, &skuBase, &sku, &barcode, &material, &brand, &depts); err != nil {
			return nil, err
		}
		id := "product:" + productID
		if sku != "" {
			id += ":" + sku
		}
		body := joinBody(brand, material)
		body = joinBody(body, depts)
		title := name
		if title == "" {
			title = skuBase
		}
		out = append(out, engine.Document{
			ID:       id,
			Kind:     engine.KindProduct,
			OrgID:    orgID,
			Title:    title,
			Subtitle: firstNonEmpty(sku, skuBase),
			Body:     body,
			SKU:      firstNonEmpty(sku, skuBase),
			Barcode:  barcode,
			Href:     "/inventory",
			Attrs: map[string]string{
				"org_code": claimOrg(orgCode),
				"material": material,
				"brand":    brand,
			},
		})
	}
	return out, rows.Err()
}

func loadLabels(ctx context.Context, tx pgx.Tx) ([]engine.Document, error) {
	rows, err := tx.Query(ctx, `
SELECT
  o.id::text,
  o.code,
  b.code,
  COALESCE(NULLIF(l.public_description, ''), p.name, '') AS title,
  COALESCE(ps.sku, '') AS sku,
  COALESCE(ps.barcode, '') AS barcode,
  COALESCE(ps.material_code, '') AS material,
  COALESCE(ps.size_code, '') AS size_code,
  COALESCE(ps.color_code, '') AS color_code,
  COALESCE(NULLIF(l.brand_label, ''), ps.brand, '') AS brand,
  COALESCE(l.price_mode, 'COMMON') AS price_mode,
  COALESCE(l.common_price, 0),
  COALESCE(l.special_price, 0),
  COALESCE(l.final_price, 0),
  COALESCE(l.department_label, '') AS dept_label
FROM store_sku_labels l
JOIN organizations o ON o.id = l.org_id
JOIN branches b ON b.id = l.branch_id
JOIN product_skus ps ON ps.id = l.sku_id
JOIN products p ON p.id = ps.product_id
WHERE l.active
ORDER BY title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []engine.Document
	for rows.Next() {
		var orgID, orgCode, branch, title, sku, barcode, material, size, color, brand, mode, dept string
		var common, special, final float64
		if err := rows.Scan(&orgID, &orgCode, &branch, &title, &sku, &barcode, &material, &size, &color, &brand, &mode, &common, &special, &final, &dept); err != nil {
			return nil, err
		}
		price := common
		switch mode {
		case "SPECIAL":
			price = special
		case "FINAL":
			price = final
		}
		body := brand
		if size != "" {
			body = joinBody(body, "talla "+size)
		}
		if color != "" {
			body = joinBody(body, color)
		}
		if material != "" {
			body = joinBody(body, material)
		}
		if dept != "" {
			body = joinBody(body, dept)
		}
		out = append(out, engine.Document{
			ID:       fmt.Sprintf("label:%s:%s", branch, sku),
			Kind:     engine.KindLabel,
			OrgID:    orgID,
			BranchID: branch,
			Title:    title,
			Subtitle: fmt.Sprintf("%s · $%.2f", sku, price),
			Body:     body,
			SKU:      sku,
			Barcode:  barcode,
			Href:     "/inventory",
			Attrs: map[string]string{
				"org_code":   claimOrg(orgCode),
				"price_mode": mode,
				"brand":      brand,
			},
		})
	}
	return out, rows.Err()
}

func loadPhotos(ctx context.Context, tx pgx.Tx) ([]engine.Document, error) {
	rows, err := tx.Query(ctx, `
SELECT
  r.org_id::text,
  COALESCE(o.code, '') AS org_code,
  COALESCE(b.code, r.branch_id::text) AS branch_code,
  r.id::text,
  COALESCE(r.title, '') AS title,
  COALESCE(r.notes, '') AS notes,
  r.created_at
FROM image_reports r
LEFT JOIN organizations o ON o.id = r.org_id
LEFT JOIN branches b ON b.id = r.branch_id
ORDER BY r.created_at DESC
LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []engine.Document
	for rows.Next() {
		var orgID, orgCode, branch, id, title, notes string
		var created time.Time
		if err := rows.Scan(&orgID, &orgCode, &branch, &id, &title, &notes, &created); err != nil {
			return nil, err
		}
		out = append(out, engine.Document{
			ID:        "photo:" + id,
			Kind:      engine.KindPhoto,
			OrgID:     orgID,
			BranchID:  branch,
			Title:     title,
			Subtitle:  "Foto · " + branch,
			Body:      notes,
			Href:      "/reports/images",
			UpdatedAt: created,
			Attrs:     map[string]string{"org_code": claimOrg(orgCode)},
		})
	}
	return out, rows.Err()
}

func claimOrg(code string) string {
	if code == "DEMO" {
		return "org_demo"
	}
	return code
}

func joinBody(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " · " + b
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
