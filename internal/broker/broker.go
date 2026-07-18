package broker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/darkrain/message-delivery/internal/config"
	"github.com/darkrain/message-delivery/internal/delivery"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Delivery = amqp.Delivery

type Broker struct {
	cfg  config.BrokerConfig
	conn *amqp.Connection
}

func Connect(cfg config.BrokerConfig, url string) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("broker: dial %s: %w", cfg.Host, err)
	}
	return &Broker{cfg: cfg, conn: conn}, nil
}

func (b *Broker) Close() error {
	if b == nil || b.conn == nil {
		return nil
	}
	return b.conn.Close()
}

func (b *Broker) Setup() error {
	ch, err := b.conn.Channel()
	if err != nil {
		return fmt.Errorf("broker: open setup channel: %w", err)
	}
	defer ch.Close()
	dlqName := b.cfg.ConsumeQueue + ".dlq"
	dlqRoutingKey := b.cfg.RoutingKeys.DeliveryRequested + ".dead"

	if err := ch.ExchangeDeclare(
		b.cfg.ExchangeName,
		b.cfg.ExchangeKind,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("broker: declare exchange: %w", err)
	}
	if _, err := ch.QueueDeclare(
		dlqName,
		true,
		false,
		false,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("broker: declare dlq: %w", err)
	}
	if err := ch.QueueBind(
		dlqName,
		dlqRoutingKey,
		b.cfg.ExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("broker: bind dlq: %w", err)
	}
	if _, err := ch.QueueDeclare(
		b.cfg.ConsumeQueue,
		true,
		false,
		false,
		false,
		amqp.Table{
			"x-dead-letter-exchange":    b.cfg.ExchangeName,
			"x-dead-letter-routing-key": dlqRoutingKey,
		},
	); err != nil {
		return fmt.Errorf("broker: declare queue: %w", err)
	}
	if err := ch.QueueBind(
		b.cfg.ConsumeQueue,
		b.cfg.RoutingKeys.DeliveryRequested,
		b.cfg.ExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("broker: bind queue: %w", err)
	}
	return nil
}

func (b *Broker) Consume() (<-chan amqp.Delivery, func() error, error) {
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("broker: open consume channel: %w", err)
	}
	if err := ch.Qos(b.cfg.PrefetchCount, 0, false); err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("broker: set qos: %w", err)
	}
	deliveries, err := ch.Consume(
		b.cfg.ConsumeQueue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("broker: consume: %w", err)
	}
	return deliveries, ch.Close, nil
}

func (b *Broker) PublishResult(ctx context.Context, event delivery.ResultEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("broker: marshal result: %w", err)
	}
	ch, err := b.conn.Channel()
	if err != nil {
		return fmt.Errorf("broker: open publish channel: %w", err)
	}
	defer ch.Close()
	return ch.PublishWithContext(ctx,
		b.cfg.ExchangeName,
		b.cfg.RoutingKeys.DeliveryResult,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         payload,
		},
	)
}
