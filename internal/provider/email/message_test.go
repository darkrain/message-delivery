package email

import (
	"bytes"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"regexp"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestMessageHasUniqueRFC5322Headers(t *testing.T) {
	seen := make(map[string]bool)
	for _, from := range []string{"no-reply@iamfree.in", "IAMFree <no-reply@iamfree.in>"} {
		for i := 0; i < 2; i++ {
			message, err := buildMessage(from, provider.Message{Recipient: "model@example.test", Subject: "Code", Body: "123456"})
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			if _, err := message.WriteTo(&wire); err != nil {
				t.Fatal(err)
			}
			parsed, err := mail.ReadMessage(&wire)
			if err != nil {
				t.Fatal(err)
			}
			id := parsed.Header.Get("Message-ID")
			if !regexp.MustCompile(`^<[0-9a-f]{32}@iamfree\.in>$`).MatchString(id) || seen[id] {
				t.Fatalf("missing, invalid or reused Message-ID: %q", id)
			}
			seen[id] = true
			if _, err := parsed.Header.Date(); err != nil {
				t.Fatalf("invalid Date: %v", err)
			}
		}
	}
}

func TestMessageRejectsInvalidSender(t *testing.T) {
	for _, from := range []string{"", "not-an-address", "sender@example.test\r\nBcc: victim@example.test"} {
		if _, err := buildMessage(from, provider.Message{}); err == nil {
			t.Fatalf("accepted invalid sender %q", from)
		}
	}
}

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
