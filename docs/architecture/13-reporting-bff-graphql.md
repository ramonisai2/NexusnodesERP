# 13 — Reporting BFF (GraphQL)

## Objetivo

BFF de **solo lectura** que agrega inventario, nómina y etiquetas/precios en una API GraphQL para la SPA y reportes.

No escribe en OLTP: llama a Inventory y Payroll por HTTP interno reenviando claims del gateway.

## Endpoint

- Gateway: `POST /graphql` → `apps/reporting-bff` (`:8084`)
- Auth: mismo JWT / headers `X-User-Id`, `X-Org-Id`, `X-Permissions`, `X-Branch-Id`

## Schema (resumen)

```graphql
type Query {
  inventorySummary(branchId: String): InventorySummary!
  stockByDepartment(branchId: String): [DepartmentStock!]!
  payrollSummary(branchId: String): PayrollSummary!
  labelPricing(branchId: String, department: String): [LabelReportRow!]!
  branchDashboard(branchId: String!): BranchDashboard!
}
```

## AuthZ

Acción OPA: `reporting.read`  
También se permite si el sujeto tiene `inventory.balance.read` o `payroll.run.read`.

## Arranque

```bash
REPORTING_ADDR=:8084 \
INVENTORY_URL=http://127.0.0.1:8082 \
PAYROLL_URL=http://127.0.0.1:8083 \
DEV_AUTH_BYPASS=true \
go run ./apps/reporting-bff/cmd/reporting
```

Gateway: `REPORTING_URL=http://127.0.0.1:8084`
