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
}

func TestNewRegistryFromConfigDisabledProviderIsUnavailable(t *testing.T) {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Phone: config.PhoneConfig{
				Adapters: map[string]config.AdapterConfig{
					"whatsapp": {"Enabled": false, "Kind": "fake", "Status": "sent"},
				},
			},
		},
	}

	registry := NewRegistryFromConfig(cfg)
	whatsapp, ok := registry.Get("whatsapp")
	if !ok {
		t.Fatal("whatsapp provider missing")
	}
	result := whatsapp.Send(context.Background(), provider.Message{})
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "provider_disabled" {
		t.Fatalf("whatsapp result = %#v", result)
	}
}
