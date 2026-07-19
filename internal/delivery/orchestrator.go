package delivery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/provider"
	templater "github.com/darkrain/message-delivery/internal/template"
)

type ResultPublisher interface {
	PublishResult(ctx context.Context, event ResultEvent) error
}

type IdempotencyStore interface {
	Seen(eventID string) bool
	Mark(eventID string)
}

type Orchestrator struct {
	cfg       *config.Config
	registry  *provider.Registry
	renderer  *templater.Renderer
	publisher ResultPublisher
	seen      IdempotencyStore
}

func NewOrchestrator(
	cfg *config.Config,
	registry *provider.Registry,
	renderer *templater.Renderer,
	publisher ResultPublisher,
	seen IdempotencyStore,
) *Orchestrator {
	return &Orchestrator{
		cfg:       cfg,
		registry:  registry,
		renderer:  renderer,
		publisher: publisher,
		seen:      seen,
	}
}

func (o *Orchestrator) Handle(ctx context.Context, event *RequestEvent) (ResultEvent, error) {
	if event == nil {
		return ResultEvent{}, fmt.Errorf("delivery: nil request event")
	}
	if o.seen != nil && o.seen.Seen(event.EventID) {
		return ResultEvent{
			Version:        event.Version,
			EventID:        newEventID(),
			Type:           EventTypeDeliveryResult,
			RequestEventID: event.EventID,
			Status:         StatusSent,
			RecipientType:  event.RecipientType,
			Recipient:      event.Recipient,
			ErrorCode:      "duplicate_ignored",
			CreatedAt:      time.Now().UTC(),
		}, nil
	}

	defaultChain := o.cfg.DefaultProviderChain(event.RecipientType)
	plan := event.ProviderPlan(defaultChain)
	if len(plan) == 0 {
		return o.publish(ctx, event, "", 0, StatusFailed, "provider_chain_empty")
	}

	locale := ""
	if event.Metadata != nil {
		locale = event.Metadata["locale"]
	}
	rendered, err := o.renderer.Render(event.Template, locale, event.Variables)
	if err != nil {
		return o.publish(ctx, event, "", 0, StatusFailed, "template_error")
	}

	for idx, providerName := range plan {
		attempt := idx + 1
		if !o.cfg.AllowedProvider(event.RecipientType, providerName) {
			if event.Delivery.SelectedProvider == providerName {
				return o.publish(ctx, event, providerName, attempt, StatusFailed, "provider_not_allowed")
			}
			continue
		}
		sender, ok := o.registry.Get(providerName)
		if !ok {
			if event.Delivery.SelectedProvider == providerName {
				return o.publish(ctx, event, providerName, attempt, StatusFailed, provider.ErrProviderNotFound.Error())
			}
			continue
		}
		result := sender.Send(ctx, provider.Message{
			EventID:       event.EventID,
			Template:      event.Template,
			Subject:       rendered.Subject,
			Body:          rendered.Body,
			ContentType:   rendered.ContentType,
			RecipientType: event.RecipientType,
			Recipient:     event.Recipient,
			Variables:     event.Variables,
			Metadata:      event.Metadata,
		})
		switch result.Status {
		case provider.StatusSent:
			if o.seen != nil {
				o.seen.Mark(event.EventID)
			}
			return o.publish(ctx, event, providerName, attempt, StatusSent, "")
		case provider.StatusUndeliverable:
			if event.Delivery.SelectedProvider == providerName && !event.Delivery.AllowFallback {
				return o.publish(ctx, event, providerName, attempt, StatusUndeliverable, result.ErrorCode)
			}
			continue
		default:
			return o.publish(ctx, event, providerName, attempt, StatusFailed, result.ErrorCode)
		}
	}

	return o.publish(ctx, event, "", len(plan), StatusUndeliverable, "all_providers_undeliverable")
}

func (o *Orchestrator) publish(ctx context.Context, request *RequestEvent, providerName string, attempt int, status string, errorCode string) (ResultEvent, error) {
	result := ResultEvent{
		Version:        request.Version,
		EventID:        newEventID(),
		Type:           EventTypeDeliveryResult,
		RequestEventID: request.EventID,
		Status:         status,
		RecipientType:  request.RecipientType,
		Recipient:      request.Recipient,
		Provider:       providerName,
		Attempt:        attempt,
		ErrorCode:      errorCode,
		CreatedAt:      time.Now().UTC(),
	}
	if o.publisher == nil {
		return result, nil
	}
	if err := o.publisher.PublishResult(ctx, result); err != nil {
		return result, err
	}
	return result, nil
}

func newEventID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("event-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
