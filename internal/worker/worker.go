package worker

import (
	"context"
	"log"

	"github.com/darkrain/message-delivery/internal/delivery"
	amqp "github.com/rabbitmq/amqp091-go"
)

type Worker struct {
	orchestrator *delivery.Orchestrator
	deliveries   <-chan amqp.Delivery
	logger       *log.Logger
}

func New(orchestrator *delivery.Orchestrator, deliveries <-chan amqp.Delivery, logger *log.Logger) *Worker {
	return &Worker{orchestrator: orchestrator, deliveries: deliveries, logger: logger}
}

func (w *Worker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-w.deliveries:
			if !ok {
				return
			}
			w.handle(ctx, msg)
		}
	}
}

func (w *Worker) handle(ctx context.Context, msg amqp.Delivery) {
	event, err := delivery.DecodeRequest(msg.Body)
	if err != nil {
		w.logger.Printf("delivery request rejected: %v", err)
		_ = msg.Nack(false, false)
		return
	}
	result, err := w.orchestrator.Handle(ctx, event)
	if err != nil {
		w.logger.Printf("delivery request %s failed: %v", event.EventID, err)
		_ = msg.Nack(false, true)
		return
	}
	w.logger.Printf("delivery request %s result=%s provider=%s", event.EventID, result.Status, result.Provider)
	_ = msg.Ack(false)
}
