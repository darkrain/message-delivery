package telegrambot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
)

type publishedConnection struct {
	event delivery.TelegramConnectionRequestedEvent
	err   error
}

func (publisher *publishedConnection) PublishTelegramConnection(_ context.Context, event delivery.TelegramConnectionRequestedEvent) error {
	publisher.event = event
	return publisher.err
}

func TestWebhookPublishesPrivateStartCommand(t *testing.T) {
	publisher := &publishedConnection{}
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", publisher, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{"update_id":17,"message":{"text":"/start abcdefghijklmnopqrstuvwxyz1234567890_-","chat":{"id":55,"type":"private","username":"test_user"},"from":{"id":55,"username":"test_user"}}}`))
	request.Header.Set(secretTokenHeader, "test-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	if publisher.event.EventID != "telegram-bot-update-17" || publisher.event.ChatID != 55 || publisher.event.StartToken != "abcdefghijklmnopqrstuvwxyz1234567890_-" {
		t.Fatalf("event = %#v", publisher.event)
	}
}

func TestWebhookRejectsWrongSecret(t *testing.T) {
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", &publishedConnection{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{}`))
	request.Header.Set(secretTokenHeader, "wrong-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestWebhookIgnoresGroupAndNonStartMessages(t *testing.T) {
	publisher := &publishedConnection{}
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", publisher, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{"update_id":17,"message":{"text":"/start abcdefghijklmnopqrstuvwxyz1234567890_-","chat":{"id":-55,"type":"group"},"from":{"id":55}}}`))
	request.Header.Set(secretTokenHeader, "test-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || publisher.event.EventID != "" {
		t.Fatalf("status=%d event=%#v", response.Code, publisher.event)
	}
}
