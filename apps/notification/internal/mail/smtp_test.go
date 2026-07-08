package mail_test

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ramonisai2/NexusnodesERP/apps/notification/internal/mail"
)

func TestParseRecipients(t *testing.T) {
	got := mail.ParseRecipients("a@demo.local, b@demo.local;c@demo.local")
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
}

func TestRenderIncludesDetails(t *testing.T) {
	text, htmlBody := mail.Render(mail.TemplateInput{
		EventType: "PayrollRunApproved",
		Title:     "Nómina aprobada",
		Summary:   "La corrida quedó aprobada.",
		Details:   map[string]string{"Sucursal": "br_norte", "Total": "1200"},
	})
	if !strings.Contains(text, "br_norte") || !strings.Contains(htmlBody, "Nómina aprobada") {
		t.Fatalf("render missing content text=%q html=%q", text, htmlBody)
	}
}

func TestSMTPSenderAgainstFakeServer(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err.Error()
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		r := bufio.NewReader(conn)
		w := bufio.NewWriter(conn)
		write := func(s string) {
			_, _ = w.WriteString(s + "\r\n")
			_ = w.Flush()
		}
		write("220 nexus-test ESMTP")
		var data strings.Builder
		inData := false
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					done <- err.Error()
				} else {
					done <- data.String()
				}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			upper := strings.ToUpper(line)
			switch {
			case strings.HasPrefix(upper, "EHLO") || strings.HasPrefix(upper, "HELO"):
				write("250-localhost")
				write("250 OK")
			case strings.HasPrefix(upper, "MAIL FROM:"):
				write("250 OK")
			case strings.HasPrefix(upper, "RCPT TO:"):
				write("250 OK")
			case upper == "DATA":
				write("354 End data with <CR><LF>.<CR><LF>")
				inData = true
			case inData && line == ".":
				write("250 OK")
				inData = false
			case upper == "QUIT":
				write("221 Bye")
				done <- data.String()
				return
			default:
				if inData {
					data.WriteString(line + "\n")
				} else {
					write("250 OK")
				}
			}
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	sender := mail.NewSMTPSender(host, port, "nexus@demo.local")
	err = sender.Send(mail.Message{
		To:      []string{"ops@demo.local"},
		Subject: "Prueba Mailhog",
		Text:    "Hola Nexus",
		HTML:    "<p>Hola Nexus</p>",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	select {
	case raw := <-done:
		if !strings.Contains(raw, "Prueba Mailhog") || !strings.Contains(raw, "Hola Nexus") {
			t.Fatalf("unexpected smtp payload: %q", raw)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting fake smtp")
	}
}
