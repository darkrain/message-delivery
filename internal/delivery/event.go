package delivery

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	EventTypeDeliveryRequested           = "message.delivery.requested"
	EventTypeDeliveryResult              = "message.delivery.result"
	EventTypeTelegramConnectionRequested = "notification.telegram.connection.requested"

	RecipientTypeEmail    = "email"
	RecipientTypePhone    = "phone"
	RecipientTypePush     = "push"
	RecipientTypeTelegram = "telegram"

	StatusSent          = "sent"
	StatusFailed        = "failed"
	StatusUndeliverable = "undeliverable"
)

type RequestEvent struct {
	Version       string            `json:"version"`
	EventID       string            `json:"event_id"`
	Type          string            `json:"type"`
	Source        string            `json:"source"`
	Template      string            `json:"template"`
	Purpose       string            `json:"purpose"`
	RecipientType string            `json:"recipient_type"`
	Recipient     string            `json:"recipient"`
	Variables     map[string]string `json:"variables"`
	UserID        int64             `json:"user_id"`
	CreatedAt     time.Time         `json:"created_at"`
	Delivery      DeliveryPolicy    `json:"delivery"`
	Metadata      map[string]string `json:"metadata"`
}

type DeliveryPolicy struct {
	SelectedProvider string   `json:"selected_provider,omitempty"`
	ProviderChain    []string `json:"provider_chain"`
	AllowFallback    bool     `json:"allow_fallback"`
}

type ResultEvent struct {
	Version        string    `json:"version"`
	EventID        string    `json:"event_id"`
	Type           string    `json:"type"`
	RequestEventID string    `json:"request_event_id"`
	Status         string    `json:"status"`
	RecipientType  string    `json:"recipient_type"`
	Recipient      string    `json:"recipient"`
	Provider       string    `json:"provider"`
	Attempt        int       `json:"attempt"`
	ErrorCode      string    `json:"error_code,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func DecodeRequest(data []byte) (*RequestEvent, error) {
	var event RequestEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, fmt.Errorf("delivery: decode request: %w", err)
	}
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return &event, nil
}

func (e *RequestEvent) Validate() error {
	if e.Version == "" {
		return errors.New("delivery: version is required")
	}
	if e.EventID == "" {
		return errors.New("delivery: event_id is required")
	}
	if e.Type != EventTypeDeliveryRequested {
		return fmt.Errorf("delivery: unsupported type %q", e.Type)
	}
	if e.Template == "" {
		return errors.New("delivery: template is required")
	}
	if e.RecipientType != RecipientTypeEmail && e.RecipientType != RecipientTypePhone && e.RecipientType != RecipientTypePush && e.RecipientType != RecipientTypeTelegram {
		return fmt.Errorf("delivery: unsupported recipient_type %q", e.RecipientType)
	}
	if e.Recipient == "" {
		return errors.New("delivery: recipient is required")
	}
	return nil
}

// TelegramConnectionRequestedEvent is emitted by the webhook after a user
// starts the bot. The API hashes StartToken before comparing it to the stored
// one-time connection ticket, so the raw value is never persisted.
type TelegramConnectionRequestedEvent struct {
	Version      string    `json:"version"`
	EventID      string    `json:"event_id"`
	Type         string    `json:"type"`
	Source       string    `json:"source"`
	UpdateID     int64     `json:"update_id"`
	ChatID       int64     `json:"chat_id"`
	ChatUsername string    `json:"chat_username,omitempty"`
	StartToken   string    `json:"start_token"`
	CreatedAt    time.Time `json:"created_at"`
}

func (e TelegramConnectionRequestedEvent) Validate() error {
	if e.Version == "" || e.EventID == "" || e.Source == "" {
		return errors.New("telegram connection event identity is required")
	}
	if e.Type != EventTypeTelegramConnectionRequested {
		return fmt.Errorf("telegram connection event has unsupported type %q", e.Type)
	}
	if e.UpdateID <= 0 || e.ChatID <= 0 || e.StartToken == "" {
		return errors.New("telegram connection event payload is invalid")
	}
	return nil
}

func (e *RequestEvent) ProviderPlan(defaultChain []string) []string {
	if e.Delivery.SelectedProvider != "" {
		if !e.Delivery.AllowFallback {
			return []string{e.Delivery.SelectedProvider}
		}
		seen := map[string]struct{}{e.Delivery.SelectedProvider: {}}
		plan := []string{e.Delivery.SelectedProvider}
		for _, provider := range append(e.Delivery.ProviderChain, defaultChain...) {
			if provider == "" {
				continue
			}
			if _, ok := seen[provider]; ok {
				continue
			}
			seen[provider] = struct{}{}
			plan = append(plan, provider)
		}
		return plan
	}
	if len(e.Delivery.ProviderChain) > 0 {
		return append([]string(nil), e.Delivery.ProviderChain...)
	}
	return append([]string(nil), defaultChain...)
}
