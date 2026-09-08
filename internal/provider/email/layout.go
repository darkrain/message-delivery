package email

import (
	"fmt"
	"html"
	"strings"
)

// The layout is operator-owned. Body/recipient data are never interpreted as
// layout syntax: NewReplacer performs a single pass over the trusted layout.
func applyLayout(layout, body, contentType, footer string) (string, error) {
	if strings.Count(layout, "{{body}}") != 1 || strings.Count(layout, "{{footer}}") != 1 {
		return "", fmt.Errorf("invalid email layout")
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		body = `<p style="margin:0 0 16px;line-height:1.65;white-space:pre-wrap">` + html.EscapeString(body) + `</p>`
	}
	return strings.NewReplacer("{{body}}", body, "{{footer}}", footer).Replace(layout), nil
}

func (p *SMTP) WithHTMLLayout(layout string) *SMTP { p.htmlLayout = layout; return p }
