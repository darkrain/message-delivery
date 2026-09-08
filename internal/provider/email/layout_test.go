package email

import (
	"bytes"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
	tmpl "github.com/darkrain/message-delivery/internal/template"
)

func TestCommonLayoutPreservesBodyAndUnsubscribeScope(t *testing.T) {
	layout, err := os.ReadFile("../../../templates/email/layout.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"auth_verification_code", "auth_password_reset", "chat_message_notification", "notification"} {
		for _, lang := range []string{"ru", "en"} {
			item := config.TemplateConfig{Subject: map[string]string{lang: "Subject"}, HtmlBodyFile: map[string]string{lang: "email/bodies/" + key + "." + lang + ".html"}}
			if key == "notification" {
				item = config.TemplateConfig{Subject: map[string]string{lang: "Subject"}, TextBody: map[string]string{lang: "Plain <script> & {{message}}"}}
			}
			r := tmpl.NewRenderer(config.TemplatesConfig{BaseDir: "../../../templates", DefaultLocale: lang, Items: map[string]config.TemplateConfig{key: item}})
			rendered, err := r.Render(key, lang, map[string]string{"code": "123456", "ttl_sec": "300", "sender": "Example <sender>", "message": "Example <message>"})
			if err != nil {
				t.Fatal(err)
			}
			msg := provider.Message{Template: key, Subject: rendered.Subject, Body: rendered.Body, ContentType: rendered.ContentType, Recipient: "fixture@example.invalid"}
			if key == "notification" || key == "chat_message_notification" {
				msg.Metadata = map[string]string{"email_unsubscribe_url": "https://example.test/unsubscribe/fixture", "email_unsubscribe_label": "Unsubscribe"}
			}
			m, err := buildMessage("no-reply@example.test", msg, string(layout))
			if err != nil {
				t.Fatal(err)
			}
			var raw bytes.Buffer
			m.WriteTo(&raw)
			parsed, err := mail.ReadMessage(&raw)
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(quotedprintable.NewReader(parsed.Body))
			if err != nil {
				t.Fatal(err)
			}
			html := string(data)
			if strings.Count(html, "email-logo.png") != 1 || !strings.Contains(html, "background:#191c25") || !strings.HasPrefix(parsed.Header.Get("Content-Type"), "text/html") {
				t.Fatal("missing common HTML layout")
			}
			if strings.HasPrefix(key, "auth_") {
				if !strings.Contains(html, "123456") || !strings.Contains(html, "300") {
					t.Fatal("code or expiry lost")
				}
				if strings.Contains(html, "unsubscribe") || parsed.Header.Get("List-Unsubscribe") != "" {
					t.Fatal("security email has unsubscribe")
				}
			} else if !strings.Contains(html, "https://example.test/unsubscribe/fixture") || parsed.Header.Get("List-Unsubscribe") == "" {
				t.Fatal("notification unsubscribe missing")
			}
			if strings.Contains(html, "<message>") || strings.Contains(html, "<sender>") || strings.Contains(html, "<script>") {
				t.Fatal("variables were not escaped")
			}
			if dir := os.Getenv("EMAIL_LAYOUT_CAPTURE_DIR"); dir != "" {
				if err := os.WriteFile(filepath.Join(dir, key+"."+lang+".html"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestCommonLayoutIsNotAppliedTwiceOrReinterpolated(t *testing.T) {
	body, err := applyLayout("<main>{{body}}</main><footer>{{footer}}</footer>", "{{footer}} <b>x</b>", "text/plain", "FOOTER")
	if err != nil || !strings.Contains(body, "{{footer}} &lt;b&gt;x&lt;/b&gt;") {
		t.Fatal("body interpreted as layout")
	}
	msg := provider.Message{Template: "notification_custom_html", Body: "<div>Already branded</div>", ContentType: "text/html"}
	m, err := buildMessage("sender@example.test", msg, "<main>{{body}}{{footer}}</main>")
	if err != nil {
		t.Fatal(err)
	}
	var raw bytes.Buffer
	m.WriteTo(&raw)
	if strings.Contains(raw.String(), "<main>") {
		t.Fatal("double wrapper")
	}
	if _, err := applyLayout("{{body}}{{body}}", "x", "text/plain", ""); err == nil {
		t.Fatal("invalid layout accepted")
	}
}
