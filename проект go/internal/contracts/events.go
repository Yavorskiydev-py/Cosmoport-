// Package contracts описывает сообщения, которыми обмениваются сервисы через RabbitMQ.
// Это общий "договор" между основным сервисом (api) и сервисом обработки (mission-control).
package contracts

import "time"

const (
	// QueueOrderCreated — очередь, в которую api публикует новые заявки на полёт.
	QueueOrderCreated = "orders.created"
	// QueueOrderStatus — очередь, в которую mission-control публикует смену статусов.
	QueueOrderStatus = "orders.status"
)

// OrderCreated — событие "поступила новая заявка на полёт" (api -> mission-control).
type OrderCreated struct {
	OrderID     int64     `json:"order_id"`
	UserID      int64     `json:"user_id"`
	Destination string    `json:"destination"`
	Passengers  int       `json:"passengers"`
	CreatedAt   time.Time `json:"created_at"`
}

// OrderStatusChanged — событие "статус заявки изменился" (mission-control -> api).
type OrderStatusChanged struct {
	OrderID   int64     `json:"order_id"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason,omitempty"`
	ChangedAt time.Time `json:"changed_at"`
}
