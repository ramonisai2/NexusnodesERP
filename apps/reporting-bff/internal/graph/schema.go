package graph

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/ramonisai2/NexusnodesERP/apps/reporting-bff/internal/upstream"
	"github.com/ramonisai2/NexusnodesERP/packages/go/authz"
)

type Services struct {
	Upstream *upstream.Client
	OPA      *authz.Client
}

func NewSchema(svc *Services) (graphql.Schema, error) {
	stockLineType := graphql.NewObject(graphql.ObjectConfig{
		Name: "StockLine",
		Fields: graphql.Fields{
			"sku":          &graphql.Field{Type: graphql.String},
			"productName":  &graphql.Field{Type: graphql.String},
			"warehouseId":  &graphql.Field{Type: graphql.String},
			"onHand":       &graphql.Field{Type: graphql.Float},
			"reserved":     &graphql.Field{Type: graphql.Float},
			"placements":   &graphql.Field{Type: graphql.NewList(graphql.String)},
			"departments":  &graphql.Field{Type: graphql.NewList(graphql.String)},
		},
	})

	departmentStockType := graphql.NewObject(graphql.ObjectConfig{
		Name: "DepartmentStock",
		Fields: graphql.Fields{
			"departmentCode": &graphql.Field{Type: graphql.String},
			"departmentName": &graphql.Field{Type: graphql.String},
			"skuCount":       &graphql.Field{Type: graphql.Int},
			"totalOnHand":    &graphql.Field{Type: graphql.Float},
			"lines":          &graphql.Field{Type: graphql.NewList(stockLineType)},
		},
	})

	inventorySummaryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "InventorySummary",
		Fields: graphql.Fields{
			"branchId":       &graphql.Field{Type: graphql.String},
			"skuCount":       &graphql.Field{Type: graphql.Int},
			"totalOnHand":    &graphql.Field{Type: graphql.Float},
			"totalReserved":  &graphql.Field{Type: graphql.Float},
			"warehouseCount": &graphql.Field{Type: graphql.Int},
			"topSkus":        &graphql.Field{Type: graphql.NewList(stockLineType)},
		},
	})

	payrollRunType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PayrollRunReport",
		Fields: graphql.Fields{
			"id":          &graphql.Field{Type: graphql.String},
			"periodLabel": &graphql.Field{Type: graphql.String},
			"branchId":    &graphql.Field{Type: graphql.String},
			"status":      &graphql.Field{Type: graphql.String},
			"totalAmount": &graphql.Field{Type: graphql.Float},
			"preparedBy":  &graphql.Field{Type: graphql.String},
			"approvedBy":  &graphql.Field{Type: graphql.String},
		},
	})

	payrollSummaryType := graphql.NewObject(graphql.ObjectConfig{
		Name: "PayrollSummary",
		Fields: graphql.Fields{
			"branchId":      &graphql.Field{Type: graphql.String},
			"runCount":      &graphql.Field{Type: graphql.Int},
			"inReviewCount": &graphql.Field{Type: graphql.Int},
			"approvedCount": &graphql.Field{Type: graphql.Int},
			"totalAmount":   &graphql.Field{Type: graphql.Float},
			"runs":          &graphql.Field{Type: graphql.NewList(payrollRunType)},
		},
	})

	labelRowType := graphql.NewObject(graphql.ObjectConfig{
		Name: "LabelReportRow",
		Fields: graphql.Fields{
			"storeDisplayName":  &graphql.Field{Type: graphql.String},
			"sku":               &graphql.Field{Type: graphql.String},
			"materialCode":      &graphql.Field{Type: graphql.String},
			"barcode":           &graphql.Field{Type: graphql.String},
			"sizeCode":          &graphql.Field{Type: graphql.String},
			"colorCode":         &graphql.Field{Type: graphql.String},
			"brand":             &graphql.Field{Type: graphql.String},
			"publicDescription": &graphql.Field{Type: graphql.String},
			"departmentLabel":   &graphql.Field{Type: graphql.String},
			"priceMode":         &graphql.Field{Type: graphql.String},
			"effectivePrice":    &graphql.Field{Type: graphql.Float},
			"currency":          &graphql.Field{Type: graphql.String},
			"onHand":            &graphql.Field{Type: graphql.Float},
		},
	})

	dashboardType := graphql.NewObject(graphql.ObjectConfig{
		Name: "BranchDashboard",
		Fields: graphql.Fields{
			"branchId":  &graphql.Field{Type: graphql.String},
			"inventory": &graphql.Field{Type: inventorySummaryType},
			"payroll":   &graphql.Field{Type: payrollSummaryType},
			"labels":    &graphql.Field{Type: graphql.NewList(labelRowType)},
			"byDepartment": &graphql.Field{Type: graphql.NewList(departmentStockType)},
		},
	})

	rootQuery := graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"inventorySummary": &graphql.Field{
				Type: inventorySummaryType,
				Args: graphql.FieldConfigArgument{
					"branchId": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: svc.resolveInventorySummary,
			},
			"stockByDepartment": &graphql.Field{
				Type: graphql.NewList(departmentStockType),
				Args: graphql.FieldConfigArgument{
					"branchId": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: svc.resolveStockByDepartment,
			},
			"payrollSummary": &graphql.Field{
				Type: payrollSummaryType,
				Args: graphql.FieldConfigArgument{
					"branchId": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: svc.resolvePayrollSummary,
			},
			"labelPricing": &graphql.Field{
				Type: graphql.NewList(labelRowType),
				Args: graphql.FieldConfigArgument{
					"branchId":   &graphql.ArgumentConfig{Type: graphql.String},
					"department": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: svc.resolveLabelPricing,
			},
			"branchDashboard": &graphql.Field{
				Type: dashboardType,
				Args: graphql.FieldConfigArgument{
					"branchId": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: svc.resolveBranchDashboard,
			},
		},
	})

	return graphql.NewSchema(graphql.SchemaConfig{Query: rootQuery})
}

func subjectFromParams(p graphql.ResolveParams) (authz.Subject, error) {
	sub, ok := p.Context.Value(subjectKey{}).(authz.Subject)
	if !ok {
		return authz.Subject{}, fmt.Errorf("missing subject")
	}
	return sub, nil
}

type subjectKey struct{}

func SubjectContextKey() subjectKey { return subjectKey{} }

func (s *Services) allowReporting(p graphql.ResolveParams, branchID string) (authz.Subject, error) {
	subject, err := subjectFromParams(p)
	if err != nil {
		return authz.Subject{}, err
	}
	allow, err := s.OPA.Allow(p.Context, authz.Input{
		Subject:  subject,
		Action:   "reporting.read",
		Resource: map[string]any{"branch_id": branchID, "org_id": subject.OrgID},
	})
	if err != nil {
		return authz.Subject{}, err
	}
	if !allow {
		return authz.Subject{}, fmt.Errorf("forbidden")
	}
	return subject, nil
}

func argString(p graphql.ResolveParams, name string) string {
	if v, ok := p.Args[name].(string); ok {
		return v
	}
	return ""
}

func (s *Services) resolveInventorySummary(p graphql.ResolveParams) (any, error) {
	branchID := argString(p, "branchId")
	subject, err := s.allowReporting(p, branchID)
	if err != nil {
		return nil, err
	}
	balances, err := s.Upstream.ListBalances(p.Context, subject, branchID, "", "")
	if err != nil {
		return nil, err
	}
	return buildInventorySummary(branchID, balances), nil
}

func (s *Services) resolveStockByDepartment(p graphql.ResolveParams) (any, error) {
	branchID := argString(p, "branchId")
	subject, err := s.allowReporting(p, branchID)
	if err != nil {
		return nil, err
	}
	balances, err := s.Upstream.ListBalances(p.Context, subject, branchID, "", "")
	if err != nil {
		return nil, err
	}
	deps, err := s.Upstream.ListDepartments(p.Context, subject, branchID)
	if err != nil {
		return nil, err
	}
	return buildDepartmentStock(deps, balances), nil
}

func (s *Services) resolvePayrollSummary(p graphql.ResolveParams) (any, error) {
	branchID := argString(p, "branchId")
	subject, err := s.allowReporting(p, branchID)
	if err != nil {
		return nil, err
	}
	runs, err := s.Upstream.ListPayrollRuns(p.Context, subject, branchID)
	if err != nil {
		return nil, err
	}
	return buildPayrollSummary(branchID, runs), nil
}

func (s *Services) resolveLabelPricing(p graphql.ResolveParams) (any, error) {
	branchID := argString(p, "branchId")
	department := argString(p, "department")
	subject, err := s.allowReporting(p, branchID)
	if err != nil {
		return nil, err
	}
	labels, err := s.Upstream.ListLabels(p.Context, subject, branchID, department)
	if err != nil {
		return nil, err
	}
	return mapLabels(labels), nil
}

func (s *Services) resolveBranchDashboard(p graphql.ResolveParams) (any, error) {
	branchID := argString(p, "branchId")
	subject, err := s.allowReporting(p, branchID)
	if err != nil {
		return nil, err
	}
	balances, err := s.Upstream.ListBalances(p.Context, subject, branchID, "", "")
	if err != nil {
		return nil, err
	}
	deps, err := s.Upstream.ListDepartments(p.Context, subject, branchID)
	if err != nil {
		return nil, err
	}
	runs, err := s.Upstream.ListPayrollRuns(p.Context, subject, branchID)
	if err != nil {
		return nil, err
	}
	labels, err := s.Upstream.ListLabels(p.Context, subject, branchID, "")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"branchId":     branchID,
		"inventory":    buildInventorySummary(branchID, balances),
		"payroll":      buildPayrollSummary(branchID, runs),
		"labels":       mapLabels(labels),
		"byDepartment": buildDepartmentStock(deps, balances),
	}, nil
}

func buildInventorySummary(branchID string, balances []upstream.Balance) map[string]any {
	warehouses := map[string]struct{}{}
	var totalOnHand, totalReserved float64
	top := make([]map[string]any, 0, len(balances))
	for _, b := range balances {
		warehouses[b.WarehouseID] = struct{}{}
		totalOnHand += b.OnHand
		totalReserved += b.Reserved
		top = append(top, mapStockLine(b))
	}
	if len(top) > 5 {
		// keep first 5 by appearance; good enough for demo summary
		top = top[:5]
	}
	return map[string]any{
		"branchId":       branchID,
		"skuCount":       len(balances),
		"totalOnHand":    totalOnHand,
		"totalReserved":  totalReserved,
		"warehouseCount": len(warehouses),
		"topSkus":        top,
	}
}

func buildDepartmentStock(deps []upstream.Department, balances []upstream.Balance) []map[string]any {
	out := make([]map[string]any, 0, len(deps)+1)
	for _, d := range deps {
		lines := make([]map[string]any, 0)
		var total float64
		seen := map[string]struct{}{}
		for _, b := range balances {
			match := false
			for _, code := range b.Departments {
				if code == d.Code {
					match = true
					break
				}
			}
			if !match {
				continue
			}
			if _, ok := seen[b.SKUID]; ok {
				continue
			}
			seen[b.SKUID] = struct{}{}
			lines = append(lines, mapStockLine(b))
			total += b.OnHand
		}
		out = append(out, map[string]any{
			"departmentCode": d.Code,
			"departmentName": d.Name,
			"skuCount":       len(lines),
			"totalOnHand":    total,
			"lines":          lines,
		})
	}
	return out
}

func buildPayrollSummary(branchID string, runs []upstream.PayrollRun) map[string]any {
	var total float64
	inReview, approved := 0, 0
	rows := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		total += r.TotalAmount
		switch r.Status {
		case "IN_REVIEW":
			inReview++
		case "APPROVED":
			approved++
		}
		rows = append(rows, map[string]any{
			"id":          r.ID,
			"periodLabel": r.PeriodLabel,
			"branchId":    r.BranchID,
			"status":      r.Status,
			"totalAmount": r.TotalAmount,
			"preparedBy":  r.PreparedBy,
			"approvedBy":  r.ApprovedBy,
		})
	}
	return map[string]any{
		"branchId":      branchID,
		"runCount":      len(runs),
		"inReviewCount": inReview,
		"approvedCount": approved,
		"totalAmount":   total,
		"runs":          rows,
	}
}

func mapLabels(labels []upstream.Label) []map[string]any {
	out := make([]map[string]any, 0, len(labels))
	for _, l := range labels {
		row := map[string]any{
			"storeDisplayName":  l.StoreDisplayName,
			"sku":               l.SKU,
			"materialCode":      l.MaterialCode,
			"barcode":           l.Barcode,
			"sizeCode":          l.SizeCode,
			"colorCode":         l.ColorCode,
			"brand":             l.Brand,
			"publicDescription": l.PublicDescription,
			"departmentLabel":   l.DepartmentLabel,
			"priceMode":         l.PriceMode,
			"currency":          l.Currency,
		}
		if l.EffectivePrice != nil {
			row["effectivePrice"] = *l.EffectivePrice
		}
		if l.OnHand != nil {
			row["onHand"] = *l.OnHand
		}
		out = append(out, row)
	}
	return out
}

func mapStockLine(b upstream.Balance) map[string]any {
	name := b.ProductName
	if name == "" {
		name = b.SKU
	}
	return map[string]any{
		"sku":         b.SKU,
		"productName": name,
		"warehouseId": b.WarehouseID,
		"onHand":      b.OnHand,
		"reserved":    b.Reserved,
		"placements":  b.Placements,
		"departments": b.Departments,
	}
}
