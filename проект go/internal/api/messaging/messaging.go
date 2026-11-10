// Package messaging связывает основной сервис с RabbitMQ:
//   - Publisher отправляет в mission-control события о новых заявках;
//   - StatusConsumer получает от mission-control изменения статусов и применяет их.
package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"cosmoport/internal/api/domain"
	"cosmoport/internal/broker"
	"cosmoport/internal/contracts"
)

// Publisher реализует service.EventPublisher поверх RabbitMQ.
type Publisher struct {
	mq *broker.Client
}

// NewPublisher создаёт публикатор событий.
func NewPublisher(mq *broker.Client) *Publisher {
	return &Publisher{mq: mq}
}

// PublishOrderCreated отправляет событие о новой заявке в очередь orders.created.
func (p *Publisher) PublishOrderCreated(ctx context.Context, event contracts.OrderCreated) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return p.mq.Publish(ctx, contracts.QueueOrderCreated, event)
}

// StatusApplier применяет изменение статуса (реализуется service.OrderService).
type StatusApplier interface {
	ApplyStatus(ctx context.Context, event contracts.OrderStatusChanged) error
}

// StatusConsumer читает очередь orders.status.
type StatusConsumer struct {
	mq      *broker.Client
	applier StatusApplier
	log     *slog.Logger
}

// NewStatusConsumer создаёт потребителя статусов.
func NewStatusConsumer(mq *broker.Client, applier StatusApplier, log *slog.Logger) *StatusConsumer {
	return &StatusConsumer{mq: mq, applier: applier, log: log}
}

// Run потребляет сообщения, пока не отменён ctx. Текущее сообщение всегда дообрабатывается
// до конца, после чего канал закрывается — это часть graceful shutdown.
func (c *StatusConsumer) Run(ctx context.Context) error {
	deliveries, ch, err := c.mq.Consume(contracts.QueueOrderStatus, "api-status-consumer", 50)
	if err != nil {
		return err
	}
	defer ch.Close()
	c.log.Info("status consumer started", "queue", contracts.QueueOrderStatus)

	for {
		select {
		case <-ctx.Done():
			c.log.Info("status consumer stopped")
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("status deliveries channel closed")
			}
			c.handle(d)
		}
	}
}

func (c *StatusConsumer) handle(d amqp.Delivery) {
	var event contracts.OrderStatusChanged
	if err := json.Unmarshal(d.Body, &event); err != nil {
		c.log.Error("drop malformed status message", "error", err)
		_ = d.Nack(false, false)
		return
	}

	// Отдельный контекст: сообщение дообрабатывается даже во время остановки сервиса.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := c.applier.ApplyStatus(ctx, event)
	switch {
	case err == nil:
		_ = d.Ack(false)
	case errors.Is(err, domain.ErrValidation), errors.Is(err, domain.ErrNotFound):
		c.log.Warn("skip status update", "order_id", event.OrderID, "status", event.Status, "error", err)
		_ = d.Ack(false)
	default:
		c.log.Error("apply status failed, requeue", "order_id", event.OrderID, "error", err)
		time.Sleep(500 * time.Millisecond)
		_ = d.Nack(false, true)
	}
}
