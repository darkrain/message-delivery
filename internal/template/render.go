package template

import (
	"fmt"
	"strings"

	"github.com/darkrain/message-delivery/internal/config"
)

type Renderer struct {
	cfg config.TemplatesConfig
}

type Rendered struct {
	Subject string
	Body    string
}

func NewRenderer(cfg config.TemplatesConfig) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) Render(templateKey string, locale string, variables map[string]string) (Rendered, error) {
	item, ok := r.cfg.Items[templateKey]
	if !ok {
		return Rendered{}, fmt.Errorf("template: %s not found", templateKey)
	}
	for _, key := range item.RequiredVariables {
		if _, ok := variables[key]; !ok {
			return Rendered{}, fmt.Errorf("template: variable %s is required", key)
		}
	}
	if locale == "" {
		locale = r.cfg.DefaultLocale
	}
	subject := localized(item.Subject, locale, r.cfg.DefaultLocale)
	body := localized(item.Body, locale, r.cfg.DefaultLocale)
	if body == "" {
		return Rendered{}, fmt.Errorf("template: %s body is empty", templateKey)
	}
	return Rendered{
		Subject: replaceVariables(subject, variables),
		Body:    replaceVariables(body, variables),
	}, nil
}

func localized(values map[string]string, locale string, fallback string) string {
	if values == nil {
		return ""
	}
	if v := values[locale]; v != "" {
		return v
	}
	return values[fallback]
}

func replaceVariables(value string, variables map[string]string) string {
	result := value
	for key, variable := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", variable)
	}
	return result
}
