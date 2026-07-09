package secure

import (
	"strings"
	"testing"
)

func TestPlainTextStripsTagsAndScripts(t *testing.T) {
	in := `Hola <script>alert(1)</script> mundo <b>x</b>`
	out := PlainText(in)
	if strings.Contains(out, "<") || strings.Contains(strings.ToLower(out), "script") {
		t.Fatalf("expected tags stripped, got %q", out)
	}
	if !strings.Contains(out, "Hola") || !strings.Contains(out, "mundo") {
		t.Fatalf("expected text kept, got %q", out)
	}
}

func TestSafeURL(t *testing.T) {
	if SafeURL("javascript:alert(1)") != "" {
		t.Fatal("javascript rejected")
	}
	if SafeURL("data:text/html,hi") != "" {
		t.Fatal("data rejected")
	}
	got := SafeURL("https://cdn.example.com/a.png?x=1#frag")
	if got == "" || strings.Contains(got, "#") {
		t.Fatalf("https url expected without fragment, got %q", got)
	}
	if SafeURL("ftp://x") != "" {
		t.Fatal("ftp rejected")
	}
}

func TestSafeCSSColor(t *testing.T) {
	if SafeCSSColor("#1A5C3A", "") != "#1a5c3a" {
		t.Fatal("hex normalize")
	}
	if SafeCSSColor("red", "#abc") != "#abc" {
		t.Fatal("fallback")
	}
	if SafeCSSColor("expression(alert(1))", "#000") != "#000" {
		t.Fatal("expression rejected")
	}
}

func TestLooksLikeInjection(t *testing.T) {
	cases := []string{
		`<script>alert(1)</script>`,
		`"><img src=x onerror=alert(1)>`,
		`1 OR 1=1`,
		`'; DROP TABLE users;--`,
		`UNION SELECT password FROM users`,
	}
	for _, c := range cases {
		if !LooksLikeInjection(c) {
			t.Fatalf("expected injection for %q", c)
		}
	}
	if LooksLikeInjection("Caja frágil — no apilar") {
		t.Fatal("benign text flagged")
	}
}
