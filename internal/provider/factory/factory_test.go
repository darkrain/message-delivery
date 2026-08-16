package factory

import (
	"context"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
)

func TestNewRegistryFromConfigBuildsFakeProviders(t *testing.T) {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Email: config.ChannelConfig{
				Adapters: map[string]config.AdapterConfig{
					"fake-email": {"Enabled": true, "Kind": "fake", "Status": "sent"},
				},
			},
			Phone: config.PhoneConfig{
				Adapters: map[string]config.AdapterConfig{
					"telegram": {"Enabled": true, "Kind": "fake", "Status": "undeliverable"},
				},
			},
			Push: config.ChannelConfig{
				Adapters: map[string]config.AdapterConfig{
					"fake-push": {"Enabled": true, "Kind": "fake", "Status": "sent"},
				},
			},
		},
	}

	registry := NewRegistryFromConfig(cfg)
	emailProvider, ok := registry.Get("fake-email")
	if !ok {
		t.Fatal("fake-email provider missing")
	}
	if result := emailProvider.Send(context.Background(), provider.Message{}); result.Status != provider.StatusSent {
		t.Fatalf("fake-email result = %#v", result)
	}

	telegramProvider, ok := registry.Get("telegram")
	if !ok {
		t.Fatal("telegram provider missing")
	}
	if result := telegramProvider.Send(context.Background(), provider.Message{}); result.Status != provider.StatusUndeliverable {
		t.Fatalf("telegram result = %#v", result)
	}

	pushProvider, ok := registry.Get("fake-push")
	if !ok {
		t.Fatal("fake-push provider missing")
	}
	if result := pushProvider.Send(context.Background(), provider.Message{}); result.Status != provider.StatusSent {
		t.Fatalf("push result = %#v", result)
	}
}

func TestNewRegistryFromConfigDisabledProviderIsUnavailable(t *testing.T) {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Phone: config.PhoneConfig{
				Adapters: map[string]config.AdapterConfig{
					"disabled-phone": {"Enabled": false, "Kind": "fake", "Status": "sent"},
				},
			},
		},
	}

	registry := NewRegistryFromConfig(cfg)
	disabledPhone, ok := registry.Get("disabled-phone")
	if !ok {
		t.Fatal("disabled-phone provider missing")
	}
	result := disabledPhone.Send(context.Background(), provider.Message{})
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "provider_disabled" {
		t.Fatalf("disabled-phone result = %#v", result)
	}
}

func TestNewRegistryFromConfigBuildsSMTPFromEnv(t *testing.T) {
	t.Setenv("SMTP_USERNAME", "user@example.com")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "user@example.com")
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Email: config.ChannelConfig{
				Adapters: map[string]config.AdapterConfig{
					"smtp": {
						"Enabled":     true,
						"Kind":        "smtp",
						"Host":        "smtp.example.com",
						"Port":        587,
						"Security":    "starttls",
						"AuthHost":    "smtp.example.com",
						"TimeoutSec":  5,
						"UsernameEnv": "SMTP_USERNAME",
						"PasswordEnv": "SMTP_PASSWORD",
						"FromEnv":     "SMTP_FROM",
					},
				},
			},
		},
	}

	registry := NewRegistryFromConfig(cfg)
	if _, ok := registry.Get("smtp"); !ok {
		t.Fatal("smtp provider missing")
	}
}
