// Package broker — тонкая обёртка над RabbitMQ (github.com/rabbitmq/amqp091-go).
// Умеет подключаться с повторными попытками, объявлять очереди, надёжно публиковать
// JSON-сообщения (с подтверждениями publisher confirms) и потреблять сообщения.
package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Client держит одно соединение с RabbitMQ и отдельный канал для публикации.
type Client struct {
	conn  *amqp.Connection
	pubCh *amqp.Channel
	mu    sync.Mutex // amqp-канал нельзя использовать для публикации из нескольких горутин одновременно
	log   *slog.Logger
}

// Connect подключается к RabbitMQ, повторяя попытки, пока брокер не станет доступен
// или не будет отменён контекст.
func Connect(ctx context.Context, url string, log *slog.Logger) (*Client, error) {
	const maxAttempts = 30
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		conn, err := amqp.Dial(url)
		if err == nil {
			ch, err := conn.Channel()
			if err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("open publish channel: %w", err)
			}
			if err := ch.Confirm(false); err != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("enable publisher confirms: %w", err)
			}
			log.Info("connected to rabbitmq", "attempt", attempt)
			return &Client{conn: conn, pubCh: ch, log: log}, nil
		}
		lastErr = err
		log.Warn("rabbitmq is not ready, retrying", "attempt", attempt, "error", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, fmt.Errorf("connect to rabbitmq: %w", lastErr)
}

// DeclareQueues объявляет durable-очереди (переживают перезапуск брокера).
func (c *Client) DeclareQueues(names ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range names {
		if _, err := c.pubCh.QueueDeclare(name, true, false, false, false, nil); err != nil {
			return fmt.Errorf("declare queue %q: %w", name, err)
		}
	}
	return nil
}

// Publish сериализует v в JSON и публикует в очередь, дожидаясь подтверждения брокера.
func (c *Client) Publish(ctx context.Context, queue string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	c.mu.Lock()
	confirm, err := c.pubCh.PublishWithDeferredConfirmWithContext(ctx, "", queue, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("publish to %q: %w", queue, err)
	}

	ok, err := confirm.WaitContext(ctx)
	if err != nil {
		return fmt.Errorf("wait confirm from %q: %w", queue, err)
	}
	if !ok {
		return errors.New("message was not acknowledged by broker")
	}
	return nil
}

// Consume открывает отдельный канал и начинает потребление очереди с ручным подтверждением.
// prefetch ограничивает число неподтверждённых сообщений, которые брокер отдаёт потребителю.
// Возвращённый канал нужно закрыть после окончания работы.
func (c *Client) Consume(queue, consumerTag string, prefetch int) (<-chan amqp.Delivery, *amqp.Channel, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("open consume channel: %w", err)
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("set qos: %w", err)
	}
	deliveries, err := ch.Consume(queue, consumerTag, false, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, nil, fmt.Errorf("consume %q: %w", queue, err)
	}
	return deliveries, ch, nil
}

// Close закрывает канал публикации и соединение.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.pubCh.Close()
	return c.conn.Close()
}
