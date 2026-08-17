package telegrambot

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
)

func TestSendMessage(t *testing.T) {
	var request sendMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bottest-token/sendMessage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer server.Close()

	sender := New("telegram-bot", server.URL, "test-token", "https://app.example.test", time.Second, config.TelegramBotPresentation{
		NotificationFooter: map[string]string{"ru": "iamfree · уведомления"},
		OpenActionLabel:    map[string]string{"ru": "Открыть в iamfree"},
	})
	result := sender.Send(context.Background(), provider.Message{
		EventID: "event-1", Recipient: "12345", Subject: "Fallback subject", Body: "Fallback body",
		Metadata: map[string]string{
			"locale": "ru", "telegram_event_type": "chat_message_received", "telegram_icon": "chat", "telegram_priority": "info",
			"telegram_title": "Anna & Co", "telegram_body": "Hello <team>", "telegram_target_path": "/models?open_chat=1",
		},
	})
	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
	if request.ChatID != 12345 || request.ParseMode != "HTML" || request.Text != "💬 <b>Новое сообщение от Anna &amp; Co</b>\n\nHello &lt;team&gt;\n\n<i>iamfree · уведомления</i>" {
		t.Fatalf("request = %#v", request)
	}
	if !request.DisableWebPagePreview || request.LinkPreviewOptions == nil || !request.LinkPreviewOptions.IsDisabled {
		t.Fatalf("preview options = %#v", request)
	}
	if request.ReplyMarkup == nil || len(request.ReplyMarkup.InlineKeyboard) != 1 || request.ReplyMarkup.InlineKeyboard[0][0] != (inlineKeyboardButton{Text: "Открыть в iamfree", URL: "https://app.example.test/models?open_chat=1"}) {
		t.Fatalf("reply markup = %#v", request.ReplyMarkup)
	}
}

func TestSendFormatsWelcome(t *testing.T) {
	var request sendMessageRequest
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	sender := New("telegram-bot", server.URL, "test-token", "", time.Second, config.TelegramBotPresentation{
		WelcomeTitle: map[string]string{"ru": "Добро пожаловать в iamfree"},
		WelcomeBody:  map[string]string{"ru": "Уведомления подключены."},
	})
	result := sender.Send(context.Background(), provider.Message{
		Recipient: "12345", Metadata: map[string]string{"telegram_presentation": "welcome", "locale": "ru"},
	})
	if result.Status != provider.StatusSent {
		t.Fatalf("result = %#v", result)
	}
	if request.Text != "👋 <b>Добро пожаловать в iamfree</b>\n\nУведомления подключены." || request.ReplyMarkup != nil {
		t.Fatalf("request = %#v", request)
	}
}

func TestSendMarksBlockedChatUndeliverable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"ok":false,"description":"Forbidden: bot was blocked by the user"}`))
	}))
	defer server.Close()

	result := New("telegram-bot", server.URL, "test-token", "", time.Second, config.TelegramBotPresentation{}).Send(context.Background(), provider.Message{Recipient: "12345", Body: "Hello"})
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "telegram_bot_chat_unavailable" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSendRejectsInvalidChat(t *testing.T) {
	result := New("telegram-bot", "", "test-token", "", time.Second, config.TelegramBotPresentation{}).Send(context.Background(), provider.Message{Recipient: "invalid"})
	if result.Status != provider.StatusUndeliverable || result.ErrorCode != "telegram_bot_chat_invalid" {
		t.Fatalf("result = %#v", result)
	}
}
