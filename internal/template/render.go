package template

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"

	"github.com/darkrain/message-delivery/internal/config"
)

type Renderer struct {
	cfg config.TemplatesConfig
}

type Rendered struct {
	Subject     string
	Body        string
	ContentType string
}

func NewRenderer(cfg config.TemplatesConfig) *Renderer {
	return &Renderer{cfg: cfg}
}

func (r *Renderer) Render(templateKey string, locale string, variables map[string]string) (Rendered, error) {
	item, ok := r.cfg.Items[templateKey]
	if !ok && strings.HasPrefix(templateKey, "notification_") {
		item, ok = r.cfg.Items["notification"]
	}
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
	body, contentType, err := r.body(item, locale)
	if err != nil {
		return Rendered{}, err
	}
	if body == "" {
		return Rendered{}, fmt.Errorf("template: %s body is empty", templateKey)
	}
	escapeBody := strings.HasPrefix(strings.ToLower(contentType), "text/html")
	return Rendered{
		Subject:     replaceVariables(subject, variables),
		Body:        replaceVariables(body, variables, escapeBody),
		ContentType: contentType,
	}, nil
}

func (r *Renderer) body(item config.TemplateConfig, locale string) (string, string, error) {
	contentType := item.ContentType
	if contentType == "" {
		contentType = "text/plain; charset=UTF-8"
	}
	if body := localized(item.HtmlBody, locale, r.cfg.DefaultLocale); body != "" {
		if item.ContentType == "" {
			contentType = "text/html; charset=UTF-8"
		}
		return body, contentType, nil
	}
	if path := localized(item.HtmlBodyFile, locale, r.cfg.DefaultLocale); path != "" {
		body, err := r.readTemplateFile(path)
		if err != nil {
			return "", "", err
		}
		if item.ContentType == "" {
			contentType = "text/html; charset=UTF-8"
		}
		return body, contentType, nil
	}
	if body := localized(item.TextBody, locale, r.cfg.DefaultLocale); body != "" {
		return body, contentType, nil
	}
	if path := localized(item.TextBodyFile, locale, r.cfg.DefaultLocale); path != "" {
		body, err := r.readTemplateFile(path)
		if err != nil {
			return "", "", err
		}
		return body, contentType, nil
	}
	return localized(item.Body, locale, r.cfg.DefaultLocale), contentType, nil
}

func (r *Renderer) readTemplateFile(path string) (string, error) {
	resolved, err := r.resolveTemplateFile(path)
	if err != nil {
		return "", err
	}
	// #nosec G304 -- resolved path is constrained to Templates.BaseDir.
	data, err := os.ReadFile(resolved)
	if err != nil {
		return "", fmt.Errorf("template: read file %q: %w", resolved, err)
	}
	return string(data), nil
}

func (r *Renderer) resolveTemplateFile(path string) (string, error) {
	if r.cfg.BaseDir == "" {
		return "", fmt.Errorf("template: BaseDir is required for file templates")
	}
	base, err := filepath.Abs(filepath.Clean(r.cfg.BaseDir))
	if err != nil {
		return "", fmt.Errorf("template: resolve base dir: %w", err)
	}
	resolved := filepath.Clean(path)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(base, resolved)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return "", fmt.Errorf("template: resolve file %q: %w", path, err)
	}
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		return "", fmt.Errorf("template: check file %q: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("template: file %q escapes BaseDir", path)
	}
	return resolved, nil
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

func replaceVariables(value string, variables map[string]string, escapeHTML ...bool) string {
	result := value
	escape := len(escapeHTML) > 0 && escapeHTML[0]
	for key, variable := range variables {
		if escape {
			variable = html.EscapeString(variable)
		}
		result = strings.ReplaceAll(result, "{{"+key+"}}", variable)
	}
	return result
}
