package telegrambot

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
	"github.com/darkrain/message-delivery/internal/provider"
)

type publishedConnection struct {
	event delivery.TelegramConnectionRequestedEvent
	err   error
}

type welcomeSender struct {
	message provider.Message
	result  provider.Result
}

func (sender *welcomeSender) Name() string { return "telegram-bot" }

func (sender *welcomeSender) Send(_ context.Context, message provider.Message) provider.Result {
	sender.message = message
	return sender.result
}

func (publisher *publishedConnection) PublishTelegramConnection(_ context.Context, event delivery.TelegramConnectionRequestedEvent) error {
	publisher.event = event
	return publisher.err
}

func TestWebhookPublishesPrivateStartCommand(t *testing.T) {
	publisher := &publishedConnection{}
	welcome := &welcomeSender{result: provider.Result{Status: provider.StatusSent}}
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", publisher, welcome, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/bot/webhook", strings.NewReader(`{"update_id":17,"message":{"text":"/start abcdefghijklmnopqrstuvwxyz1234567890_-","chat":{"id":55,"type":"private","username":"test_user"},"from":{"id":55,"username":"test_user","language_code":"ru-RU"}}}`))
	request.Header.Set(secretTokenHeader, "test-secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	ticket := "abcdefghijklmnopqrstuvwxyz1234567890_-"
	hash := sha256.Sum256([]byte(ticket))
	if publisher.event.EventID != "telegram-bot-update-17" || publisher.event.ChatID != 55 || publisher.event.StartTokenHash != fmt.Sprintf("%x", hash[:]) {
		t.Fatalf("event = %#v", publisher.event)
	}
	if welcome.message.Recipient != "55" || welcome.message.Metadata["telegram_presentation"] != "welcome" || welcome.message.Metadata["locale"] != "ru-RU" {
		t.Fatalf("welcome = %#v", welcome.message)
	}
}

func TestWebhookRejectsWrongSecret(t *testing.T) {
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", &publishedConnection{}, &welcomeSender{}, nil)
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
	handler, err := NewWebhookHandler(config.TelegramBotConfig{Enabled: true, MaxBodyBytes: 1024, PublishTimeoutSec: 1}, "test-secret", publisher, &welcomeSender{}, nil)
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
