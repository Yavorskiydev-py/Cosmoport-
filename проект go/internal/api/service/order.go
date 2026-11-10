package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cosmoport/internal/api/domain"
	"cosmoport/internal/api/metrics"
	"cosmoport/internal/contracts"
)

const (
	maxPassengers   = 10
	defaultPageSize = 50
	maxPageSize     = 200
)

// catalog — каталог направлений космопорта.
var catalog = []domain.Destination{
	{Code: "iss", Name: "Международная космическая станция", DistanceKm: 408, FlightDays: 1, PricePerSeat: 55_000},
	{Code: "moon", Name: "Луна, база «Море Спокойствия»", DistanceKm: 384_400, FlightDays: 3, PricePerSeat: 150_000},
	{Code: "mars", Name: "Марс, кратер Езеро", DistanceKm: 225_000_000, FlightDays: 210, PricePerSeat: 1_200_000},
	{Code: "europa", Name: "Европа, спутник Юпитера", DistanceKm: 628_300_000, FlightDays: 600, PricePerSeat: 3_500_000},
	{Code: "titan", Name: "Титан, спутник Сатурна", DistanceKm: 1_200_000_000, FlightDays: 1100, PricePerSeat: 6_000_000},
}

// OrderService — бизнес-логика заявок на полёт (ЛР4, ЛР5) и обработка статусов из mission-control.
type OrderService struct {
	orders    OrderRepository
	publisher EventPublisher
	log       *slog.Logger
}

// NewOrderService создаёт сервис заявок.
func NewOrderService(orders OrderRepository, publisher EventPublisher, log *slog.Logger) *OrderService {
	return &OrderService{orders: orders, publisher: publisher, log: log}
}

// Destinations возвращает каталог направлений.
func (s *OrderService) Destinations() []domain.Destination {
	out := make([]domain.Destination, len(catalog))
	copy(out, catalog)
	return out
}

func findDestination(code string) (domain.Destination, bool) {
	for _, d := range catalog {
		if d.Code == code {
			return d, true
		}
	}
	return domain.Destination{}, false
}

// Create создаёт заявку на полёт для пользователя userID и отправляет событие в mission-control.
func (s *OrderService) Create(ctx context.Context, userID int64, destination string, passengers int) (domain.Order, error) {
	dest, ok := findDestination(destination)
	if !ok {
		return domain.Order{}, fmt.Errorf("%w: unknown destination %q, see GET /destinations", domain.ErrValidation, destination)
	}
	if passengers < 1 || passengers > maxPassengers {
		return domain.Order{}, fmt.Errorf("%w: passengers must be between 1 and %d", domain.ErrValidation, maxPassengers)
	}

	order, err := s.orders.Create(ctx, domain.Order{
		UserID:      userID,
		Destination: dest.Code,
		Passengers:  passengers,
		Price:       dest.PricePerSeat * int64(passengers),
		Status:      domain.StatusCreated,
	})
	if err != nil {
		return domain.Order{}, err
	}
	metrics.OrdersCreated.WithLabelValues(order.Destination).Inc()

	// Заявка уже сохранена, поэтому ошибка брокера не должна "ронять" запрос:
	// пользователь получит заявку в статусе created, а ошибка попадёт в логи и метрики.
	err = s.publisher.PublishOrderCreated(ctx, contracts.OrderCreated{
		OrderID:     order.ID,
		UserID:      order.UserID,
		Destination: order.Destination,
		Passengers:  order.Passengers,
		CreatedAt:   order.CreatedAt,
	})
	if err != nil {
		metrics.EventsPublished.WithLabelValues(contracts.QueueOrderCreated, "failure").Inc()
		s.log.Error("publish order created event", "order_id", order.ID, "error", err)
	} else {
		metrics.EventsPublished.WithLabelValues(contracts.QueueOrderCreated, "success").Inc()
	}
	return order, nil
}

// List возвращает заявки пользователя userID (постранично).
func (s *OrderService) List(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error) {
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if offset < 0 {
		offset = 0
	}
	return s.orders.ListByUser(ctx, userID, limit, offset)
}

// Get возвращает заявку, только если она принадлежит пользователю userID.
func (s *OrderService) Get(ctx context.Context, userID, orderID int64) (domain.Order, error) {
	return s.orders.GetByID(ctx, orderID, userID)
}

// ApplyStatus применяет изменение статуса, пришедшее из mission-control через RabbitMQ.
func (s *OrderService) ApplyStatus(ctx context.Context, event contracts.OrderStatusChanged) error {
	status := domain.OrderStatus(event.Status)
	if !status.Valid() {
		return fmt.Errorf("%w: unknown status %q", domain.ErrValidation, event.Status)
	}
	if err := s.orders.UpdateStatus(ctx, event.OrderID, status, event.Reason); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("order %d not found or already finished: %w", event.OrderID, err)
		}
		return err
	}
	metrics.OrderStatusUpdates.WithLabelValues(string(status)).Inc()
	s.log.Info("order status changed",
		"order_id", event.OrderID, "status", status, "reason", event.Reason,
		"lag_ms", time.Since(event.ChangedAt).Milliseconds())
	return nil
}
