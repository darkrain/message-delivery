package factory

import (
	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
	"github.com/darkrain/message-delivery/internal/provider/email"
	"github.com/darkrain/message-delivery/internal/provider/fake"
	"github.com/darkrain/message-delivery/internal/provider/telegram"
)

func NewRegistryFromConfig(cfg *config.Config) *provider.Registry {
	registry := provider.NewRegistry()
	for name, adapter := range cfg.Providers.Email.Adapters {
		if p := buildProvider(name, adapter, deliveryRecipientEmail); p != nil {
			registry.Register(p)
		}
	}
	for name, adapter := range cfg.Providers.Phone.Adapters {
		if p := buildProvider(name, adapter, deliveryRecipientPhone); p != nil {
			registry.Register(p)
		}
	}
	return registry
}

const (
	deliveryRecipientEmail = "email"
	deliveryRecipientPhone = "phone"
)

func buildProvider(name string, adapter config.AdapterConfig, recipientType string) provider.Provider {
	if !adapter.Bool("Enabled") {
		return provider.NewUnavailable(name, "provider_disabled")
	}
	switch adapter.String("Kind") {
	case "fake":
		status := adapter.String("Status")
		if status == "" {
			status = provider.StatusSent
		}
		return fake.New(name, status)
	case "smtp":
		return email.NewSMTP(
			name,
			adapter.String("Host"),
			adapter.Int("Port"),
			adapter.EnvString("UsernameEnv"),
			adapter.EnvString("PasswordEnv"),
			adapter.String("From"),
		)
	case "telegram-gateway":
		return telegram.NewGateway(name, adapter.String("BaseURL"), adapter.EnvString("ApiTokenEnv"))
	case "":
		if recipientType == deliveryRecipientPhone {
			return provider.NewUnavailable(name, name+"_not_configured")
		}
		return provider.NewUnavailable(name, "provider_not_configured")
	default:
		return provider.NewUnavailable(name, "provider_kind_unsupported")
	}
}
