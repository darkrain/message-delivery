package integration

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/darkrain/message-delivery/internal/delivery"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRabbitMQDeliveryFlow(t *testing.T) {
	if os.Getenv("MESSAGE_DELIVERY_INTEGRATION") != "1" {
		t.Skip("set MESSAGE_DELIVERY_INTEGRATION=1 to run integration tests")
	}

	host := envOrDefault("MESSAGE_DELIVERY_BROKER_HOST", "127.0.0.1:5672")
	user := envOrDefault("MESSAGE_DELIVERY_BROKER_USER", "guest")
	password := envOrDefault("RABBITMQ_PASSWORD", "guest")
	url := "amqp://" + user + ":" + password + "@" + host + "/"

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("dial RabbitMQ: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("open channel: %v", err)
	}
	defer ch.Close()

	const exchange = "messages.events"
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}

	resultQueue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatalf("declare result queue: %v", err)
	}
	if err := ch.QueueBind(resultQueue.Name, "message.delivery.result", exchange, false, nil); err != nil {
		t.Fatalf("bind result queue: %v", err)
	}
	results, err := ch.Consume(resultQueue.Name, "", true, true, false, false, nil)
	if err != nil {
		t.Fatalf("consume result queue: %v", err)
	}

	request := delivery.RequestEvent{
		Version:       "v1",
		EventID:       "integration-" + time.Now().UTC().Format("20060102150405.000000000"),
		Type:          delivery.EventTypeDeliveryRequested,
		Template:      "auth_verification_code",
		RecipientType: delivery.RecipientTypePhone,
		Recipient:     "+10000000000",
		Variables:     map[string]string{"code": "123456", "ttl_sec": "300"},
		Delivery: delivery.DeliveryPolicy{
			ProviderChain: []string{"telegram", "sms"},
			AllowFallback: true,
		},
		Metadata: map[string]string{"locale": "en"},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := ch.PublishWithContext(ctx, exchange, "message.delivery.requested", false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         payload,
	}); err != nil {
		t.Fatalf("publish request: %v", err)
	}

	select {
	case msg := <-results:
		var result delivery.ResultEvent
		if err := json.Unmarshal(msg.Body, &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if result.RequestEventID != request.EventID {
			t.Fatalf("request_event_id = %q, want %q", result.RequestEventID, request.EventID)
		}
		if result.Status != delivery.StatusSent || result.Provider != "sms" || result.Attempt != 2 {
			t.Fatalf("result = %#v", result)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for delivery result")
	}
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
