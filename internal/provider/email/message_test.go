package email

import (
	"bytes"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestUnsubscribeHeadersAndFooter(t *testing.T) {
	for _, contentType := range []string{"text/plain; charset=UTF-8", "text/html; charset=UTF-8"} {
		msg := provider.Message{Recipient: "recipient@example.test", Subject: "Subject", Body: "Body", ContentType: contentType, Metadata: map[string]string{"email_unsubscribe_url": "https://example.test/unsubscribe/opaque", "email_unsubscribe_label": "Отписаться"}}
		m, err := buildMessage("sender@example.test", msg)
		if err != nil {
			t.Fatal(err)
		}
		var raw bytes.Buffer
		if _, err := m.WriteTo(&raw); err != nil {
			t.Fatal(err)
		}
		parsed, err := mail.ReadMessage(&raw)
		if err != nil {
			t.Fatal(err)
		}
		if parsed.Header.Get("List-Unsubscribe") != "<https://example.test/unsubscribe/opaque>" || parsed.Header.Get("List-Unsubscribe-Post") != "List-Unsubscribe=One-Click" {
			t.Fatal("one-click headers missing")
		}
		body, _ := io.ReadAll(quotedprintable.NewReader(parsed.Body))
		if !strings.Contains(string(body), "https://example.test/unsubscribe/opaque") || !strings.Contains(string(body), "Отписаться") {
			t.Fatal("footer missing")
		}
	}
}

func TestUnsubscribeDoesNotAffectSecurityMailOrDuplicateBrandedFooter(t *testing.T) {
	for _, metadata := range []map[string]string{nil, {"email_unsubscribe_url": "https://example.test/unsubscribe/opaque", "email_unsubscribe_in_body": "true"}} {
		msg := provider.Message{Recipient: "recipient@example.test", Subject: "Code", Body: "123456", Metadata: metadata}
		m, err := buildMessage("sender@example.test", msg)
		if err != nil {
			t.Fatal(err)
		}
		var raw bytes.Buffer
		m.WriteTo(&raw)
		parsed, _ := mail.ReadMessage(&raw)
		body, _ := io.ReadAll(quotedprintable.NewReader(parsed.Body))
		if strings.TrimSpace(string(body)) != "123456" {
			t.Fatal("body unexpectedly altered")
		}
		if metadata == nil && parsed.Header.Get("List-Unsubscribe") != "" {
			t.Fatal("security mail has unsubscribe")
		}
	}
}

func TestUnsubscribeRejectsHeaderInjectionAndUnsafeURLs(t *testing.T) {
	for _, address := range []string{"https://example.test\r\nBcc: victim@example.test", "javascript:alert(1)", "http://example.test/u", "https://user:pass@example.test/u", "https://example.test/u>"} {
		_, err := buildMessage("sender@example.test", provider.Message{Metadata: map[string]string{"email_unsubscribe_url": address}})
		if err == nil {
			t.Fatalf("accepted unsafe URL %q", address)
		}
	}
}
