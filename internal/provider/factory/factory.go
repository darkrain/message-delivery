package factory

import (
	"time"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
	"github.com/darkrain/message-delivery/internal/provider/email"
	"github.com/darkrain/message-delivery/internal/provider/fake"
	"github.com/darkrain/message-delivery/internal/provider/telegram"
	"github.com/darkrain/message-delivery/internal/provider/webpush"
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
	for name, adapter := range cfg.Providers.Push.Adapters {
		if p := buildProvider(name, adapter, deliveryRecipientPush); p != nil {
			registry.Register(p)
		}
	}
	return registry
}

const (
	deliveryRecipientEmail = "email"
	deliveryRecipientPhone = "phone"
	deliveryRecipientPush  = "push"
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
		from := adapter.String("From")
		if from == "" {
			from = adapter.EnvString("FromEnv")
		}
		return email.NewSMTP(
			name,
			adapter.String("Host"),
			adapter.Int("Port"),
			adapter.String("AuthHost"),
			adapter.EnvString("UsernameEnv"),
			adapter.EnvString("PasswordEnv"),
			from,
			adapter.String("Security"),
			timeout(adapter, 15*time.Second),
		)
	case "telegram-gateway":
		return telegram.NewGateway(name, adapter.String("BaseURL"), adapter.EnvString("ApiTokenEnv"), timeout(adapter, 10*time.Second))
	case "webpush":
		return webpush.New(
			name,
			adapter.EnvString("VAPIDPublicKeyEnv"),
			adapter.EnvString("VAPIDPrivateKeyEnv"),
			adapter.String("VAPIDSubscriber"),
			timeout(adapter, 10*time.Second),
		)
	case "":
		if recipientType == deliveryRecipientPhone || recipientType == deliveryRecipientPush {
			return provider.NewUnavailable(name, name+"_not_configured")
		}
		return provider.NewUnavailable(name, "provider_not_configured")
	default:
		return provider.NewUnavailable(name, "provider_kind_unsupported")
	}
}

func timeout(adapter config.AdapterConfig, fallback time.Duration) time.Duration {
	seconds := adapter.Int("TimeoutSec")
	if seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
