.PHONY: deps test-go test-go-pg run-gateway run-inventory run-payroll run-web tidy migrate

DATABASE_URL ?= postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable
OPA_URL ?= http://127.0.0.1:8181

deps:
	cd packages/go/db && go mod tidy
	cd packages/go/authz && go mod tidy
	cd apps/gateway && go mod tidy
	cd apps/inventory && go mod tidy
	cd apps/payroll && go mod tidy
	pnpm install

tidy: deps

migrate:
	bash infra/scripts/migrate.sh up

test-go:
	cd packages/go/authz && go test ./...
	cd apps/gateway && go test ./...
	cd apps/inventory && go test ./...
	cd apps/payroll && go test ./...

test-go-pg:
	DATABASE_URL='$(DATABASE_URL)' go test ./apps/inventory/internal/store ./apps/payroll/internal/store -count=1

run-gateway:
	DEV_AUTH_BYPASS=true OPA_URL='$(OPA_URL)' go run ./apps/gateway/cmd/gateway

run-inventory:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' go run ./apps/inventory/cmd/inventory

run-payroll:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' go run ./apps/payroll/cmd/payroll

run-web:
	pnpm --filter @nexus/web dev
