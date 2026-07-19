package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadExampleConfig(t *testing.T) {
	cfg, err := Load("../../message-delivery.example.json")
	if err != nil {
		t.Fatalf("Load example config: %v", err)
	}
	if cfg.Broker.ExchangeName != "messages.events" {
		t.Errorf("ExchangeName = %q", cfg.Broker.ExchangeName)
	}
	if !strings.HasSuffix(cfg.Templates.BaseDir, "templates") {
		t.Errorf("Templates.BaseDir = %q, want templates suffix", cfg.Templates.BaseDir)
	}
	if got := cfg.DefaultProviderChain("phone"); len(got) != 3 {
		t.Errorf("phone chain len = %d, want 3", len(got))
	}
	if !cfg.AllowedProvider("phone", "telegram") {
		t.Error("telegram should be allowed for phone")
	}
	if cfg.AllowedProvider("email", "telegram") {
		t.Error("telegram should not be allowed for email")
	}
}

func TestLoadTelegramExampleConfig(t *testing.T) {
	cfg, err := Load("../../message-delivery.telegram.example.json")
	if err != nil {
		t.Fatalf("Load telegram example config: %v", err)
	}
	if got := cfg.DefaultProviderChain("phone"); len(got) != 1 || got[0] != "telegram" {
		t.Errorf("phone chain = %#v, want telegram only", got)
	}
	if !cfg.AllowedProvider("phone", "telegram") {
		t.Error("telegram should be allowed for phone")
	}
	if cfg.AllowedProvider("phone", "sms") {
		t.Error("sms should not be allowed in telegram example config")
	}
}

func TestLoadSMTPExampleConfig(t *testing.T) {
	cfg, err := Load("../../message-delivery.smtp.example.json")
	if err != nil {
		t.Fatalf("Load smtp example config: %v", err)
	}
	if cfg.Providers.Email.DefaultProvider != "smtp" {
		t.Errorf("email default provider = %q, want smtp", cfg.Providers.Email.DefaultProvider)
	}
	if !cfg.AllowedProvider("email", "smtp") {
		t.Error("smtp should be allowed for email")
	}
	if cfg.AllowedProvider("email", "fake-email") {
		t.Error("fake-email should not be allowed in smtp example config")
	}
	adapter := cfg.Providers.Email.Adapters["smtp"]
	if adapter.String("Host") != "smtp.yandex.com" {
		t.Errorf("smtp host = %q, want smtp.yandex.com", adapter.String("Host"))
	}
	if adapter.String("Security") != "tls" {
		t.Errorf("smtp security = %q, want tls", adapter.String("Security"))
	}
	if adapter.String("AuthHost") != "smtp.yandex.com" {
		t.Errorf("smtp auth host = %q, want smtp.yandex.com", adapter.String("AuthHost"))
	}
	if adapter.Int("TimeoutSec") <= 0 {
		t.Error("smtp timeout must be configured")
	}
}

func TestLoadEnvOverridesBroker(t *testing.T) {
	t.Setenv("MESSAGE_DELIVERY_BROKER_HOST", "rabbitmq:5672")
	t.Setenv("MESSAGE_DELIVERY_BROKER_PASSWORD", "secret")

	cfg, err := Load("../../message-delivery.example.json")
	if err != nil {
		t.Fatalf("Load example config: %v", err)
	}
	if cfg.Broker.Host != "rabbitmq:5672" {
		t.Errorf("Broker.Host = %q", cfg.Broker.Host)
	}
	if cfg.Broker.Password != "secret" {
		t.Errorf("Broker.Password env override failed")
	}
}

func TestLoadPasswordEnv(t *testing.T) {
	t.Setenv("RABBITMQ_PASSWORD", "from-env")
	os.Unsetenv("MESSAGE_DELIVERY_BROKER_PASSWORD")

	cfg, err := Load("../../message-delivery.example.json")
	if err != nil {
		t.Fatalf("Load example config: %v", err)
	}
	if cfg.Broker.Password != "from-env" {
		t.Errorf("Broker.Password = %q, want from-env", cfg.Broker.Password)
	}
}
