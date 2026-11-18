package template

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
)

// auth-service warns the previous e-mail after a confirmed contact change
// (theGHub1/docs#156, SEC-05). The template carries no code.
func TestRenderContactChangedFromExampleConfig(t *testing.T) {
	for _, file := range []string{"message-delivery.example.json", "message-delivery.smtp.example.json", "message-delivery.telegram.example.json"} {
		cfg, err := config.Load(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		cfg.Templates.BaseDir = filepath.Join("..", "..", "templates")
		renderer := NewRenderer(cfg.Templates)
		for _, locale := range []string{"ru", "en"} {
			rendered, err := renderer.Render("auth_contact_changed", locale, map[string]string{"contact_type": "email", "new_contact": "ne***@example.com"})
			if err != nil {
				t.Fatalf("%s %s: %v", file, locale, err)
			}
			if !strings.Contains(rendered.Body, "ne***@example.com") || strings.Contains(rendered.Body, "{{") || rendered.Subject == "" {
				t.Fatalf("%s %s: unexpected render %#v", file, locale, rendered)
			}
		}
		if _, err := renderer.Render("auth_contact_changed", "ru", map[string]string{}); err == nil {
			t.Fatalf("%s: new_contact must be required", file)
		}
	}
}
