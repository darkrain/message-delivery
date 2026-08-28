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

func TestRenderHTMLTemplateEscapesVariables(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"html": {
				HtmlBody: map[string]string{"en": "<p>{{name}}</p>"},
			},
		},
	})
	rendered, err := renderer.Render("html", "en", map[string]string{"name": `<script>alert("x")</script>`})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(rendered.Body, "<script>") {
		t.Fatalf("Body was not escaped: %q", rendered.Body)
	}
	if !strings.Contains(rendered.Body, "&lt;script&gt;") {
		t.Fatalf("Body = %q, want escaped script tag", rendered.Body)
	}
}

func TestRenderTextTemplateDoesNotHTMLEscapeVariables(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"text": {
				TextBody: map[string]string{"en": "Hello {{name}}"},
			},
		},
	})
	rendered, err := renderer.Render("text", "en", map[string]string{"name": "<Anna>"})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rendered.Body != "Hello <Anna>" {
		t.Fatalf("Body = %q", rendered.Body)
	}
}

func TestRenderNotificationTemplateWithoutDomainVariables(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"notification": {
				Subject:  map[string]string{"en": "New notification", "ru": "Новое уведомление"},
				TextBody: map[string]string{"en": "Open the application.", "ru": "Откройте приложение."},
			},
		},
	})
	rendered, err := renderer.Render("notification", "ru", nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if rendered.Subject != "Новое уведомление" || rendered.Body != "Откройте приложение." {
		t.Fatalf("rendered = %#v", rendered)
	}
}

func TestRenderTemplateFileRejectsBaseDirEscape(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		BaseDir:       t.TempDir(),
		Items: map[string]config.TemplateConfig{
			"escape": {
				HtmlBodyFile: map[string]string{"en": "../outside.html"},
			},
		},
	})
	_, err := renderer.Render("escape", "en", nil)
	if err == nil || !strings.Contains(err.Error(), "escapes BaseDir") {
		t.Fatalf("err = %v, want BaseDir escape error", err)
	}
}

func TestRenderTemplateFileRequiresBaseDir(t *testing.T) {
	renderer := NewRenderer(config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"file": {
				HtmlBodyFile: map[string]string{"en": "email/file.html"},
			},
		},
	})
	_, err := renderer.Render("file", "en", nil)
	if err == nil || !strings.Contains(err.Error(), "BaseDir is required") {
		t.Fatalf("err = %v, want BaseDir required error", err)
	}
}
