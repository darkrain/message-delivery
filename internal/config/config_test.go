package config

import (
	"os"
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
