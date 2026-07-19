package delivery

import (
	"encoding/json"
	"testing"
)

func TestDecodeRequestValidatesContract(t *testing.T) {
	payload := map[string]any{
		"version":        "v1",
		"event_id":       "event-1",
		"type":           EventTypeDeliveryRequested,
		"template":       "auth_verification_code",
		"recipient_type": RecipientTypePhone,
		"recipient":      "+10000000000",
	}
	data, _ := json.Marshal(payload)
	event, err := DecodeRequest(data)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if event.EventID != "event-1" {
		t.Errorf("EventID = %q", event.EventID)
	}
}

func TestDecodeRequestRejectsUnsupportedType(t *testing.T) {
	payload := []byte(`{"version":"v1","event_id":"event-1","type":"wrong","template":"t","recipient_type":"phone","recipient":"+1"}`)
	_, err := DecodeRequest(payload)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestProviderPlanSelectedWithoutFallback(t *testing.T) {
	event := RequestEvent{
		Delivery: DeliveryPolicy{
			SelectedProvider: "telegram",
			ProviderChain:    []string{"telegram", "sms"},
			AllowFallback:    false,
		},
	}
	plan := event.ProviderPlan([]string{"sms"})
	if len(plan) != 1 || plan[0] != "telegram" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestProviderPlanSelectedWithFallback(t *testing.T) {
	event := RequestEvent{
		Delivery: DeliveryPolicy{
			SelectedProvider: "backup",
			ProviderChain:    []string{"telegram", "backup", "sms"},
			AllowFallback:    true,
		},
	}
	plan := event.ProviderPlan([]string{"telegram", "backup", "sms"})
	want := []string{"backup", "telegram", "sms"}
	if len(plan) != len(want) {
		t.Fatalf("plan len = %d", len(plan))
	}
	for i := range want {
		if plan[i] != want[i] {
			t.Fatalf("plan = %#v, want %#v", plan, want)
		}
	}
}
