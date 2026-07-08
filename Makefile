.PHONY: deps test-go run-gateway run-inventory run-payroll run-web tidy

deps:
	cd apps/gateway && go mod tidy
	cd apps/inventory && go mod tidy
	cd apps/payroll && go mod tidy
	pnpm install

tidy: deps

test-go:
	cd apps/gateway && go test ./...
	cd apps/inventory && go test ./...
	cd apps/payroll && go test ./...

run-gateway:
	DEV_AUTH_BYPASS=true go run ./apps/gateway/cmd/gateway

run-inventory:
	DEV_AUTH_BYPASS=true go run ./apps/inventory/cmd/inventory

run-payroll:
	DEV_AUTH_BYPASS=true go run ./apps/payroll/cmd/payroll

run-web:
	pnpm --filter @nexus/web dev
