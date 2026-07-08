.PHONY: deps test-go test-go-pg run-gateway run-inventory run-payroll run-reporting run-reports run-relay run-notification run-web tidy migrate

DATABASE_URL ?= postgres://nexus:nexus@127.0.0.1:5432/nexus_erp?sslmode=disable
OPA_URL ?= http://127.0.0.1:8181
NATS_URL ?= nats://127.0.0.1:4222
OTEL_EXPORTER ?= none

deps:
	cd packages/go/db && go mod tidy
	cd packages/go/authz && go mod tidy
	cd packages/go/events && go mod tidy
	cd packages/go/otelx && go mod tidy
	cd apps/gateway && go mod tidy
	cd apps/inventory && go mod tidy
	cd apps/payroll && go mod tidy
	cd apps/reporting-bff && go mod tidy
	cd apps/reports && go mod tidy
	cd apps/outbox-relay && go mod tidy
	cd apps/notification && go mod tidy
	pnpm install

tidy: deps

migrate:
	bash infra/scripts/migrate.sh up

test-go:
	cd packages/go/authz && go test ./...
	cd packages/go/events && go test ./...
	cd packages/go/otelx && go test ./...
	cd packages/go/db && go test ./...
	cd apps/gateway && go test ./...
	cd apps/inventory && go test ./...
	cd apps/payroll && go test ./...
	cd apps/reporting-bff && go test ./...
	cd apps/reports && go test ./...
	cd apps/outbox-relay && go test ./...
	cd apps/notification && go test ./...

test-go-pg:
	DATABASE_URL='$(DATABASE_URL)' go test ./apps/inventory/internal/store ./apps/payroll/internal/store ./apps/gateway/internal/enrich ./packages/go/db -count=1

run-gateway:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' REPORTING_URL=http://127.0.0.1:8084 REPORTS_URL=http://127.0.0.1:8085 go run ./apps/gateway/cmd/gateway

run-inventory:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' go run ./apps/inventory/cmd/inventory

run-payroll:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' go run ./apps/payroll/cmd/payroll

run-reporting:
	DEV_AUTH_BYPASS=true OPA_URL='$(OPA_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' INVENTORY_URL=http://127.0.0.1:8082 PAYROLL_URL=http://127.0.0.1:8083 go run ./apps/reporting-bff/cmd/reporting

run-reports:
	DEV_AUTH_BYPASS=true DATABASE_URL='$(DATABASE_URL)' OPA_URL='$(OPA_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' \
	IMAGE_STORAGE_PATH=./data/image-reports IMAGE_MAX_EDGE=1280 IMAGE_JPEG_QUALITY=82 \
	go run ./apps/reports/cmd/reports

run-relay:
	DATABASE_URL='$(DATABASE_URL)' NATS_URL='$(NATS_URL)' OUTBOX_POLL_INTERVAL=1s OTEL_EXPORTER='$(OTEL_EXPORTER)' go run ./apps/outbox-relay/cmd/relay

run-notification:
	DATABASE_URL='$(DATABASE_URL)' NATS_URL='$(NATS_URL)' OTEL_EXPORTER='$(OTEL_EXPORTER)' \
	SMTP_ENABLED=true SMTP_HOST=127.0.0.1 SMTP_PORT=1025 SMTP_FROM=nexus@demo.local \
	NOTIFY_EMAIL_TO=ops@demo.nexus,analyst@demo.nexus \
	go run ./apps/notification/cmd/notification

run-web:
	pnpm --filter @nexus/web dev
