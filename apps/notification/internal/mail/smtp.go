package mail

import (
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	From    string
	To      []string
	Subject string
	Text    string
	HTML    string
}

type Sender interface {
	Send(msg Message) error
}

// SMTPSender sends email via plain SMTP (Mailhog: localhost:1025, no auth).
type SMTPSender struct {
	Addr string
	From string
}

func NewSMTPSender(host, port, from string) *SMTPSender {
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "1025"
	}
	if from == "" {
		from = "nexus@demo.local"
	}
	return &SMTPSender{Addr: net.JoinHostPort(host, port), From: from}
}

func (s *SMTPSender) Send(msg Message) error {
	from := msg.From
	if from == "" {
		from = s.From
	}
	if len(msg.To) == 0 {
		return fmt.Errorf("no recipients")
	}
	body := buildMIME(from, msg)
	return smtp.SendMail(s.Addr, nil, from, msg.To, []byte(body))
}

// LogSender writes messages to stdout (used when SMTP is disabled).
type LogSender struct{}

func NewLogSender() *LogSender { return &LogSender{} }

func (s *LogSender) Send(msg Message) error {
	fmt.Printf("email to=%v subject=%q body=%q\n", msg.To, msg.Subject, msg.Text)
	return nil
}

func buildMIME(from string, msg Message) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(msg.To, ", ") + "\r\n")
	b.WriteString("Subject: " + sanitizeHeader(msg.Subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n")
	if msg.HTML != "" {
		boundary := "nexus-boundary"
		b.WriteString("Content-Type: multipart/alternative; boundary=" + boundary + "\r\n\r\n")
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Text + "\r\n")
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.HTML + "\r\n")
		b.WriteString("--" + boundary + "--\r\n")
	} else {
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(msg.Text + "\r\n")
	}
	return b.String()
}

func sanitizeHeader(v string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, v)
}

// ParseRecipients splits comma/semicolon separated emails.
func ParseRecipients(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
