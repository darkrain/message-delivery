package email

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/provider"
)

func TestBrandFromLayoutAndEmbeddedLogo(t *testing.T) {
	brands := map[string]Brand{}
	for name, from := range map[string]string{"iamfree": "IAMFREE <no-reply@iamfree24.com>", "topmember": "IAM TopMember <no-reply@iamtopmember.com>"} {
		layout, err := os.ReadFile("../../../templates/email/brands/" + name + ".html")
		if err != nil {
			t.Fatal(err)
		}
		brands[name] = Brand{From: from, Layout: string(layout), LogoFile: "../../../templates/email/brands/" + name + ".png"}
	}
	p := (&SMTP{}).WithBrands("iamfree", brands)
	for _, name := range []string{"iamfree", "topmember", ""} {
		selected := name
		if selected == "" {
			selected = "iamfree"
		}
		for _, template := range []string{"auth_verification_code", "auth_password_reset", "notification_custom_html"} {
			metadata := map[string]string{"brand": name}
			if template == "notification_custom_html" {
				metadata["email_apply_brand_layout"] = "true"
				metadata["email_unsubscribe_url"] = "https://example.test/unsubscribe/fixture"
				metadata["email_unsubscribe_label"] = "Отписаться"
			}
			bodyHTML := "<h1>Новое уведомление</h1><p>123456</p>"
			if strings.HasPrefix(template, "auth_") {
				data, err := os.ReadFile("../../../templates/email/bodies/" + template + ".ru.html")
				if err != nil {
					t.Fatal(err)
				}
				bodyHTML = strings.NewReplacer("{{code}}", "123456", "{{ttl_sec}}", "300").Replace(string(data))
			}
			m, err := p.message(provider.Message{Template: template, Recipient: "fixture@example.invalid", Subject: "Код", Body: bodyHTML, ContentType: "text/html", Metadata: metadata})
			if err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			if _, err := m.WriteTo(&wire); err != nil {
				t.Fatal(err)
			}
			parsed, err := mail.ReadMessage(&wire)
			if err != nil {
				t.Fatal(err)
			}
			from, err := mail.ParseAddress(parsed.Header.Get("From"))
			if err != nil {
				t.Fatal(err)
			}
			expected, _ := mail.ParseAddress(brands[selected].From)
			if from.Address != expected.Address {
				t.Fatalf("wrong brand From: %s", from.Address)
			}
			if !strings.Contains(parsed.Header.Get("Message-ID"), strings.Split(expected.Address, "@")[1]) {
				t.Fatal("Message-ID brand mismatch")
			}
			mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
			if err != nil || mediaType != "multipart/related" {
				t.Fatalf("missing embedded MIME parts: %s", mediaType)
			}
			reader := multipart.NewReader(parsed.Body, params["boundary"])
			htmlPart, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			htmlBytes, err := io.ReadAll(htmlPart)
			if err != nil {
				t.Fatal(err)
			}
			body := string(htmlBytes)
			if strings.Count(body, "cid:brand-logo.png") != 1 || !strings.Contains(body, "123456") || strings.Contains(body, "iamfree.in") {
				t.Fatal("wrong branded body")
			}
			if !strings.Contains(body, strings.Split(expected.Address, "@")[1]) {
				t.Fatal("wrong brand footer")
			}
			if template != "notification_custom_html" && parsed.Header.Get("List-Unsubscribe") != "" {
				t.Fatal("OTP has unsubscribe")
			}
			logo, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			if logo.Header.Get("Content-ID") != "<brand-logo.png>" {
				t.Fatal("logo CID mismatch")
			}
			data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, logo))
			if err != nil {
				t.Fatal(err)
			}
			original, _ := os.ReadFile(brands[selected].LogoFile)
			if !bytes.Equal(data, original) {
				t.Fatal("embedded logo differs")
			}
			if dir := os.Getenv("EMAIL_BRAND_CAPTURE_DIR"); dir != "" && name != "" && template == "auth_verification_code" {
				body = strings.ReplaceAll(body, "cid:brand-logo.png", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data))
				if err := os.WriteFile(filepath.Join(dir, name+".html"), []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	if _, err := p.message(provider.Message{Metadata: map[string]string{"brand": "untrusted.example"}}); err == nil {
		t.Fatal("accepted unknown brand")
	}
}
