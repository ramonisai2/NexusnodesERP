module github.com/ramonisai2/NexusnodesERP/apps/gateway

go 1.22

require (
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/cors v1.2.1
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/google/uuid v1.6.0
	github.com/ramonisai2/NexusnodesERP/packages/go/authz v0.0.0
)

replace github.com/ramonisai2/NexusnodesERP/packages/go/authz => ../../packages/go/authz
