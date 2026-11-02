package email

import (
	"fmt"

	"github.com/darkrain/message-delivery/internal/provider"
	"gopkg.in/gomail.v2"
)

// Brands are operator-owned. Events select an allowlisted identity, never an
// arbitrary From address, logo path or layout.
type Brand struct {
	From     string
	Layout   string
	LogoFile string
}

func (p *SMTP) WithBrands(defaultBrand string, brands map[string]Brand) *SMTP {
	p.defaultBrand, p.brands = defaultBrand, brands
	return p
}

func (p *SMTP) message(msg provider.Message) (*gomail.Message, error) {
	from, layout, logo := p.from, p.htmlLayout, ""
	if len(p.brands) > 0 {
		name := msg.Metadata["brand"]
		if name == "" {
			name = p.defaultBrand
		}
		brand, ok := p.brands[name]
		if !ok {
			return nil, fmt.Errorf("unknown email brand")
		}
		from, layout, logo = brand.From, brand.Layout, brand.LogoFile
	}
	message, err := buildMessage(from, msg, layout)
	if err != nil {
		return nil, err
	}
	if logo != "" {
		message.Embed(logo, gomail.Rename("brand-logo.png"))
	}
	return message, nil
}
