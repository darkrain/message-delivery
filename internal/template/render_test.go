package template

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
)

func TestRenderTemplate(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"auth_verification_code": {
				Subject:           map[string]string{"en": "Code"},
				Body:              map[string]string{"en": "Code {{code}} expires in {{ttl_sec}}"},
				RequiredVariables: []string{"code", "ttl_sec"},
			},
		},
	})
	rendered, err := renderer.Render("auth_verification_code", "en", map[string]string{"code": "123456", "ttl_sec": "300"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(rendered.Body, "123456") {
		t.Fatalf("Body = %q", rendered.Body)
	}
	if rendered.ContentType != "text/plain; charset=UTF-8" {
		t.Fatalf("ContentType = %q", rendered.ContentType)
	}
}

func TestRenderMissingVariable(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"auth_verification_code": {
				Body:              map[string]string{"en": "Code {{code}}"},
				RequiredVariables: []string{"code"},
			},
		},
	})
	_, err := renderer.Render("auth_verification_code", "en", map[string]string{})
	if err == nil {
		t.Fatal("expected missing variable error")
	}
}

func TestRenderHTMLTemplateFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "code.ru.html"), []byte("<strong>{{code}}</strong>"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		BaseDir:       dir,
		Items: map[string]config.TemplateConfig{
			"auth_verification_code": {
				Subject:      map[string]string{"en": "Code", "ru": "Код"},
				HtmlBodyFile: map[string]string{"ru": "code.ru.html"},
			},
		},
	})
	rendered, err := renderer.Render("auth_verification_code", "ru", map[string]string{"code": "123456"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rendered.Body != "<strong>123456</strong>" {
		t.Fatalf("Body = %q", rendered.Body)
	}
	if rendered.ContentType != "text/html; charset=UTF-8" {
		t.Fatalf("ContentType = %q", rendered.ContentType)
	}
}
