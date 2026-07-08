package mail

import (
	"fmt"
	"html"
	"strings"
)

type TemplateInput struct {
	EventType string
	Title     string
	Summary   string
	Details   map[string]string
}

func Render(in TemplateInput) (text, htmlBody string) {
	var tb strings.Builder
	tb.WriteString(in.Title + "\n\n")
	tb.WriteString(in.Summary + "\n\n")
	if len(in.Details) > 0 {
		tb.WriteString("Detalle:\n")
		for k, v := range in.Details {
			tb.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
		}
	}
	tb.WriteString("\n— NexusERP\n")
	text = tb.String()

	var hb strings.Builder
	hb.WriteString(`<!doctype html><html><body style="font-family:Segoe UI,Arial,sans-serif;color:#1b2a24;background:#f4f7f5;padding:24px;">`)
	hb.WriteString(`<div style="max-width:560px;margin:0 auto;background:#fff;border:1px solid #d7e0db;border-radius:12px;padding:20px 22px;">`)
	hb.WriteString(`<p style="margin:0 0 4px;font-size:12px;letter-spacing:.08em;text-transform:uppercase;color:#6b7c74;">NexusERP</p>`)
	hb.WriteString(`<h1 style="margin:0 0 12px;font-size:20px;">` + html.EscapeString(in.Title) + `</h1>`)
	hb.WriteString(`<p style="margin:0 0 16px;line-height:1.45;">` + html.EscapeString(in.Summary) + `</p>`)
	if len(in.Details) > 0 {
		hb.WriteString(`<table style="width:100%;border-collapse:collapse;font-size:14px;">`)
		for k, v := range in.Details {
			hb.WriteString(`<tr><td style="padding:6px 0;color:#6b7c74;width:38%;">` + html.EscapeString(k) + `</td>`)
			hb.WriteString(`<td style="padding:6px 0;font-family:ui-monospace,Menlo,monospace;">` + html.EscapeString(v) + `</td></tr>`)
		}
		hb.WriteString(`</table>`)
	}
	hb.WriteString(`<p style="margin:18px 0 0;font-size:12px;color:#6b7c74;">Evento: ` + html.EscapeString(in.EventType) + `</p>`)
	hb.WriteString(`</div></body></html>`)
	htmlBody = hb.String()
	return text, htmlBody
}
