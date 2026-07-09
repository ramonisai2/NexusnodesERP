package middleware

import (
	"net/http"

	"github.com/ramonisai2/NexusnodesERP/packages/go/secure"
)

// Hardening applies security headers, body size limits, and JSON content-type checks.
func Hardening(maxBody int64) func(http.Handler) http.Handler {
	limit := secure.LimitBodyMiddleware(maxBody)
	return func(next http.Handler) http.Handler {
		h := secure.SecurityHeadersMiddleware(next)
		h = secure.RequireJSONMiddleware(h)
		h = limit(h)
		return h
	}
}
