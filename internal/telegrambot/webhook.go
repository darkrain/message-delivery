package telegrambot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
)

const secretTokenHeader = "X-Telegram-Bot-Api-Secret-Token"

var startCommand = regexp.MustCompile(`^/start(?:@[A-Za-z0-9_]+)?\s+([A-Za-z0-9_-]{20,64})\s*$`)

type ConnectionPublisher interface {
	PublishTelegramConnection(context.Context, delivery.TelegramConnectionRequestedEvent) error
}

type WebhookHandler struct {
	cfg       config.TelegramBotConfig
	secret    string
	publisher ConnectionPublisher
	logger    *log.Logger
}

func NewWebhookHandler(cfg config.TelegramBotConfig, secret string, publisher ConnectionPublisher, logger *log.Logger) (*WebhookHandler, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("telegram bot webhook is disabled")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, fmt.Errorf("telegram bot webhook secret is required")
	}
	if publisher == nil {
		return nil, fmt.Errorf("telegram bot webhook publisher is required")
	}
	if logger == nil {
		logger = log.Default()
	}
	return &WebhookHandler{cfg: cfg, secret: secret, publisher: publisher, logger: logger}, nil
}

func (handler *WebhookHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Header.Get(secretTokenHeader)), []byte(handler.secret)) != 1 {
		http.Error(writer, "forbidden", http.StatusForbidden)
		return
	}

	var update update
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, handler.cfg.MaxBodyBytes))
	if err := decoder.Decode(&update); err != nil {
		http.Error(writer, "invalid update", http.StatusBadRequest)
		return
	}
	event, ok := telegramConnectionEvent(update)
	if !ok {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	publishContext, cancel := context.WithTimeout(request.Context(), time.Duration(handler.cfg.PublishTimeoutSec)*time.Second)
	defer cancel()
	if err := handler.publisher.PublishTelegramConnection(publishContext, event); err != nil {
		handler.logger.Printf("telegram webhook publish update_id=%d failed: %v", update.UpdateID, err)
		http.Error(writer, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID       int64  `json:"id"`
			Type     string `json:"type"`
			Username string `json:"username"`
		} `json:"chat"`
		From struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

func telegramConnectionEvent(update update) (delivery.TelegramConnectionRequestedEvent, bool) {
	if update.UpdateID <= 0 || update.Message == nil || update.Message.Chat.Type != "private" || update.Message.Chat.ID <= 0 || update.Message.From.ID != update.Message.Chat.ID {
		return delivery.TelegramConnectionRequestedEvent{}, false
	}
	matches := startCommand.FindStringSubmatch(update.Message.Text)
	if len(matches) != 2 {
		return delivery.TelegramConnectionRequestedEvent{}, false
	}
	username := strings.TrimSpace(update.Message.Chat.Username)
	if username == "" {
		username = strings.TrimSpace(update.Message.From.Username)
	}
	event := delivery.TelegramConnectionRequestedEvent{
		Version: "1.0", EventID: fmt.Sprintf("telegram-bot-update-%d", update.UpdateID), Type: delivery.EventTypeTelegramConnectionRequested,
		Source: "message-delivery.telegram-bot", UpdateID: update.UpdateID, ChatID: update.Message.Chat.ID, ChatUsername: username,
		StartToken: matches[1], CreatedAt: time.Now().UTC(),
	}
	return event, event.Validate() == nil
}
