package secure

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// DefaultMaxBodyBytes caps JSON/API request bodies (1 MiB).
const DefaultMaxBodyBytes int64 = 1 << 20

var (
	htmlTagRe = regexp.MustCompile(`(?is)<[^>]*>`)
	scriptRe  = regexp.MustCompile(`(?is)<\s*/?\s*script\b|javascript\s*:|vbscript\s*:|data\s*:\s*text/html|on\w+\s*=`)
	sqlProbeRe = regexp.MustCompile(`(?is)(?:\bunion\b\s+\bselect\b|\bor\b\s+1\s*=\s*1|--\s|/\*|\*/|;?\s*drop\s+table\b|;?\s*insert\s+into\b|;?\s*update\s+\w+\s+set\b|;?\s*delete\s+from\b|\bxp_cmdshell\b)`)
	ctrlRe    = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
	hexColorRe = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
)

// PlainText strips HTML tags, dangerous URI schemes, and control characters.
// Use for names, notes, descriptions, and other free-text fields.
func PlainText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = htmlTagRe.ReplaceAllString(s, "")
	s = scriptRe.ReplaceAllString(s, "")
	s = ctrlRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\u2028", " ")
	s = strings.ReplaceAll(s, "\u2029", " ")
	return strings.TrimSpace(collapseSpace(s))
}

// PlainTextMax applies PlainText and truncates to max runes (not bytes).
func PlainTextMax(s string, max int) string {
	s = PlainText(s)
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}

// SafeURL allows only http/https absolute URLs (or empty). Rejects javascript:/data: etc.
func SafeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Block obvious scheme tricks before parse.
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "javascript:") ||
		strings.HasPrefix(lower, "vbscript:") ||
		strings.HasPrefix(lower, "data:") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return ""
	}
	if u.Host == "" {
		return ""
	}
	// Rebuild without userinfo / fragments that can confuse CSS url().
	u.User = nil
	u.Fragment = ""
	return u.String()
}

// SafeCSSColor allows #RGB / #RRGGBB / #RRGGBBAA only.
func SafeCSSColor(raw, fallback string) string {
	raw = strings.TrimSpace(raw)
	if hexColorRe.MatchString(raw) {
		return strings.ToLower(raw)
	}
	if fallback != "" {
		return fallback
	}
	return "#1a5c3a"
}

// LooksLikeInjection reports obvious XSS/SQLi probes in free text.
// Parameterized SQL remains the primary defense; this is a reject-early layer.
func LooksLikeInjection(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if scriptRe.MatchString(s) {
		return true
	}
	if sqlProbeRe.MatchString(s) {
		return true
	}
	return false
}

// RejectIfInjection returns an error message when input looks hostile.
func RejectIfInjection(field, value string) string {
	if LooksLikeInjection(value) {
		return field + ": potential injection rejected"
	}
	return ""
}

func collapseSpace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if r == '\n' || r == '\r' || r == '\t' {
				if !prevSpace {
					b.WriteByte(' ')
					prevSpace = true
				}
				continue
			}
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

// SecurityHeadersMiddleware adds browser hardening headers on every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-site")
		// API-oriented CSP: no script execution from responses; SPA hosts its own CSP.
		if h.Get("Content-Security-Policy") == "" {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		}
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// LimitBodyMiddleware rejects oversized bodies early (DoS / bomb prevention).
func LimitBodyMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost, http.MethodPut, http.MethodPatch:
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireJSONMiddleware rejects mutating requests without JSON content type
// (except empty-body POSTs and multipart uploads).
func RequireJSONMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			ct := r.Header.Get("Content-Type")
			if ct == "" {
				// Allow action POSTs without body (ship/receive/print).
				next.ServeHTTP(w, r)
				return
			}
			media := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
			switch media {
			case "application/json", "application/problem+json",
				"multipart/form-data", "application/octet-stream",
				"image/jpeg", "image/png", "image/webp", "image/gif":
				next.ServeHTTP(w, r)
				return
			default:
				http.Error(w, `{"error":"unsupported_media_type"}`, http.StatusUnsupportedMediaType)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
