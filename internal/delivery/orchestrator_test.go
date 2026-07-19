package delivery

import (
	"context"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
	"github.com/darkrain/message-delivery/internal/provider/fake"
	templater "github.com/darkrain/message-delivery/internal/template"
)

type capturePublisher struct {
	results []ResultEvent
}

func (p *capturePublisher) PublishResult(_ context.Context, event ResultEvent) error {
	p.results = append(p.results, event)
	return nil
}

func TestOrchestratorFallsBackAcrossProviderChain(t *testing.T) {
	telegram := fake.New("telegram", provider.StatusUndeliverable)
	backup := fake.New("backup", provider.StatusUndeliverable)
	sms := fake.New("sms", provider.StatusSent)
	pub := &capturePublisher{}

	orch := newTestOrchestrator(pub, provider.NewRegistry(telegram, backup, sms))
	result, err := orch.Handle(context.Background(), phoneEvent(""))
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Status != StatusSent || result.Provider != "sms" || result.Attempt != 3 {
		t.Fatalf("result = %#v", result)
	}
	if telegram.Count() != 1 || backup.Count() != 1 || sms.Count() != 1 {
		t.Fatalf("provider counts telegram=%d backup=%d sms=%d", telegram.Count(), backup.Count(), sms.Count())
	}
	if len(pub.results) != 1 || pub.results[0].Provider != "sms" {
		t.Fatalf("published results = %#v", pub.results)
	}
}

func TestOrchestratorSelectedProviderWithoutFallback(t *testing.T) {
	telegram := fake.New("telegram", provider.StatusUndeliverable)
	sms := fake.New("sms", provider.StatusSent)
	orch := newTestOrchestrator(&capturePublisher{}, provider.NewRegistry(telegram, sms))

	event := phoneEvent("telegram")
	event.Delivery.AllowFallback = false
	result, err := orch.Handle(context.Background(), event)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Status != StatusUndeliverable || result.Provider != "telegram" || result.Attempt != 1 {
		t.Fatalf("result = %#v", result)
	}
	if sms.Count() != 0 {
		t.Fatalf("sms should not be called")
	}
}

func TestOrchestratorRejectsNotAllowedProvider(t *testing.T) {
	pub := &capturePublisher{}
	orch := newTestOrchestrator(pub, provider.NewRegistry(fake.New("telegram", provider.StatusSent)))

	event := phoneEvent("pager")
	event.Delivery.AllowFallback = false
	result, err := orch.Handle(context.Background(), event)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if result.Status != StatusFailed || result.ErrorCode != "provider_not_allowed" {
		t.Fatalf("result = %#v", result)
	}
}

func TestOrchestratorIdempotencySkipsDuplicate(t *testing.T) {
	sms := fake.New("sms", provider.StatusSent)
	pub := &capturePublisher{}
	orch := newTestOrchestrator(pub, provider.NewRegistry(sms))
	event := phoneEvent("")
	event.Delivery.ProviderChain = []string{"sms"}

	if _, err := orch.Handle(context.Background(), event); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	result, err := orch.Handle(context.Background(), event)
	if err != nil {
		t.Fatalf("second Handle: %v", err)
	}
	if result.ErrorCode != "duplicate_ignored" {
		t.Fatalf("result = %#v", result)
	}
	if sms.Count() != 1 {
		t.Fatalf("sms count = %d, want 1", sms.Count())
	}
	if len(pub.results) != 2 || pub.results[1].ErrorCode != "duplicate_ignored" {
		t.Fatalf("published results = %#v", pub.results)
	}
}

func newTestOrchestrator(pub ResultPublisher, registry *provider.Registry) *Orchestrator {
	cfg := &config.Config{
		Providers: config.ProvidersConfig{
			Email: config.ChannelConfig{DefaultProvider: "email", AllowedProviders: []string{"email"}},
			Phone: config.PhoneConfig{
				DefaultProviderChain: []string{"telegram", "backup", "sms"},
				AllowedProviders:     []string{"telegram", "backup", "sms"},
			},
		},
		Templates: testTemplates(),
	}
	return NewOrchestrator(cfg, registry, templater.NewRenderer(cfg.Templates), pub, NewMemoryIdempotencyStore())
}

func phoneEvent(selectedProvider string) *RequestEvent {
	return &RequestEvent{
		Version:       "v1",
		EventID:       "event-1",
		Type:          EventTypeDeliveryRequested,
		Template:      "auth_verification_code",
		RecipientType: RecipientTypePhone,
		Recipient:     "+10000000000",
		Variables:     map[string]string{"code": "123456", "ttl_sec": "300"},
		Delivery: DeliveryPolicy{
			SelectedProvider: selectedProvider,
			ProviderChain:    []string{"telegram", "backup", "sms"},
			AllowFallback:    true,
		},
		Metadata: map[string]string{"locale": "en"},
	}
}

func testTemplates() config.TemplatesConfig {
	return config.TemplatesConfig{
		DefaultLocale: "en",
		Items: map[string]config.TemplateConfig{
			"auth_verification_code": {
				Subject:           map[string]string{"en": "Code"},
				Body:              map[string]string{"en": "Code {{code}} expires in {{ttl_sec}}"},
				RequiredVariables: []string{"code", "ttl_sec"},
			},
		},
	}
}
