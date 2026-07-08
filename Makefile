.PHONY: deps test-go test-go-pg run-gateway run-inventory run-payroll run-relay run-web tidy migrate

DATABASE_URL ?= postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable
OPA_URL ?= http://127.0.0.1:8181
NATS_URL ?= nats://127.0.0.1:4222

deps:
	cd packages/go/db && go mod tidy
	cd packages/go/authz && go mod tidy
	cd packages/go/events && go mod tidy
	cd apps/gateway && go mod tidy
	cd apps/inventory && go mod tidy
	cd apps/payroll && go mod tidy
	cd apps/outbox-relay && go mod tidy
	pnpm install

tidy: deps

migrate:
	bash infra/scripts/migrate.sh up

test-go:
	cd packages/go/authz && go test ./...
	cd packages/go/events && go test ./...
	cd apps/gateway && go test ./...
	cd apps/inventory && go test ./...
	cd apps/payroll && go test ./...
	cd apps/outbox-relay && go test ./...

test-go-pg:
	DATABASE_URL='$(DATABASE_URL)' go test ./apps/inventory/internal/store ./apps/payroll/internal/store ./apps/gateway/internal/enrich ./packages/go/db -count=1

run-gateway:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' go run ./apps/gateway/cmd/gateway

run-inventory:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' go run ./apps/inventory/cmd/inventory

run-payroll:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' go run ./apps/payroll/cmd/payroll

run-relay:
	DATABASE_URL='$(DATABASE_URL)' NATS_URL='$(NATS_URL)' OUTBOX_POLL_INTERVAL=1s go run ./apps/outbox-relay/cmd/relay

run-web:
	pnpm --filter @nexus/web dev
