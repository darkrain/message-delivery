package webpush

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	webpushgo "github.com/SherClockHolmes/webpush-go"
	"github.com/darkrain/message-delivery/internal/provider"
)

type Provider struct {
	name            string
	vapidPublicKey  string
	vapidPrivateKey string
	subscriber      string
	timeout         time.Duration
	client          webpushgo.HTTPClient
}

func New(name, vapidPublicKey, vapidPrivateKey, subscriber string, timeout time.Duration) *Provider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Provider{
		name: name, vapidPublicKey: strings.TrimSpace(vapidPublicKey), vapidPrivateKey: strings.TrimSpace(vapidPrivateKey), subscriber: strings.TrimSpace(subscriber), timeout: timeout,
		client: &http.Client{Timeout: timeout},
	}
}

func (p *Provider) Name() string { return p.name }

func (p *Provider) Send(ctx context.Context, message provider.Message) provider.Result {
	if p.vapidPublicKey == "" || p.vapidPrivateKey == "" || p.subscriber == "" {
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "webpush_not_configured"}
	}
	p256dh, authKey := message.Metadata["push_p256dh"], message.Metadata["push_auth"]
	if strings.TrimSpace(message.Recipient) == "" || strings.TrimSpace(p256dh) == "" || strings.TrimSpace(authKey) == "" {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_subscription_invalid"}
	}
	payload, err := json.Marshal(map[string]string{
		"title":       first(message.Metadata["push_title"], message.Subject),
		"body":        first(message.Metadata["push_body"], message.Body),
		"target_path": message.Metadata["push_target_path"],
		"tag":         first(message.Metadata["push_tag"], message.EventID),
	})
	if err != nil {
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_payload_invalid"}
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	response, err := webpushgo.SendNotificationWithContext(ctx, payload, &webpushgo.Subscription{
		Endpoint: message.Recipient,
		Keys:     webpushgo.Keys{P256dh: p256dh, Auth: authKey},
	}, &webpushgo.Options{
		HTTPClient: p.client, Subscriber: p.subscriber, VAPIDPublicKey: p.vapidPublicKey, VAPIDPrivateKey: p.vapidPrivateKey, TTL: 60,
	})
	if err != nil {
		if ctx.Err() != nil {
			return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_timeout"}
		}
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_send_failed"}
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices:
		return provider.Result{Status: provider.StatusSent}
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		return provider.Result{Status: provider.StatusUndeliverable, ErrorCode: "webpush_subscription_expired"}
	case response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError:
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_service_unavailable"}
	default:
		return provider.Result{Status: provider.StatusFailed, ErrorCode: "webpush_rejected"}
	}
}

func first(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
