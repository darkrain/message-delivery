package main

import (
	"testing"

	"github.com/darkrain/message-delivery/internal/delivery"
)

func TestBuildRequestPhone(t *testing.T) {
	event, err := buildRequest(options{
		recipientType:    delivery.RecipientTypePhone,
		recipient:        "+15551234567",
		template:         "auth_verification_code",
		purpose:          "manual_test",
		source:           "manual-client",
		eventID:          "manual-1",
		code:             "123456",
		ttlSec:           "300",
		locale:           "ru",
		selectedProvider: "telegram",
		providerChain:    "telegram, whatsapp, sms",
		allowFallback:    true,
		variablesJSON:    `{"name":"Anna","attempt":2}`,
		metadataJSON:     `{"device_uid":"device-1"}`,
		userID:           42,
	})
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if event.Type != delivery.EventTypeDeliveryRequested || event.EventID != "manual-1" {
		t.Fatalf("event = %#v", event)
	}
	if event.Variables["code"] != "123456" || event.Variables["attempt"] != "2" {
		t.Fatalf("variables = %#v", event.Variables)
	}
	if event.Metadata["locale"] != "ru" || event.Metadata["device_uid"] != "device-1" {
		t.Fatalf("metadata = %#v", event.Metadata)
	}
	if event.Delivery.SelectedProvider != "telegram" || len(event.Delivery.ProviderChain) != 3 {
		t.Fatalf("delivery = %#v", event.Delivery)
	}
}

func TestParseOptionsRequiresRecipient(t *testing.T) {
	_, err := parseOptions([]string{"--recipient-type", "phone"})
	if err == nil {
		t.Fatal("expected recipient validation error")
	}
}

func TestParseStringMapRejectsNestedValues(t *testing.T) {
	_, err := parseStringMap(`{"nested":{"a":"b"}}`)
	if err == nil {
		t.Fatal("expected nested value validation error")
	}
}

func TestSplitCSV(t *testing.T) {
	got := splitCSV("telegram, whatsapp,, sms ")
	want := []string{"telegram", "whatsapp", "sms"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}
