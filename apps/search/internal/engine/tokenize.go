package engine

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Tokenize turns free text into searchable terms (lowercase, accent-folded, alphanumeric).
func Tokenize(s string) []string {
	s = fold(s)
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tok := b.String()
		b.Reset()
		if len(tok) < 1 {
			return
		}
		out = append(out, tok)
		// Prefix shards for typeahead / partial SKU & barcode matches.
		if len(tok) >= 3 && len(tok) <= 24 {
			runes := []rune(tok)
			max := len(runes)
			if max > 8 {
				max = 8
			}
			for i := 3; i <= max; i++ {
				out = append(out, string(runes[:i]))
			}
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return dedupe(out)
}

func fold(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã':
			b.WriteByte('a')
		case 'é', 'è', 'ë', 'ê':
			b.WriteByte('e')
		case 'í', 'ì', 'ï', 'î':
			b.WriteByte('i')
		case 'ó', 'ò', 'ö', 'ô', 'õ':
			b.WriteByte('o')
		case 'ú', 'ù', 'ü', 'û':
			b.WriteByte('u')
		case 'ñ':
			b.WriteByte('n')
		case 'ç':
			b.WriteByte('c')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func dedupe(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, t := range in {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func hasPrefixToken(haystack, needle string) bool {
	if needle == "" {
		return false
	}
	h := fold(haystack)
	n := fold(needle)
	if strings.Contains(h, n) {
		return true
	}
	// Fast path for short queries against compact fields.
	if utf8.RuneCountInString(n) >= 2 && (strings.HasPrefix(h, n) || strings.Contains(h, " "+n)) {
		return true
	}
	return false
}
