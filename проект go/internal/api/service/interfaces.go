// Package service — слой бизнес-логики (второй слой чистой архитектуры).
//
// Сервисы ничего не знают об HTTP и о конкретной БД: они зависят только от интерфейсов,
// объявленных в этом файле. Конкретные реализации (PostgreSQL, RabbitMQ, JWT)
// подставляются при сборке приложения в пакете app.
package service

import (
	"context"
	"time"

	"cosmoport/internal/api/domain"
	"cosmoport/internal/contracts"
)

// GreetingRepository отдаёт приветствие (ЛР1).
type GreetingRepository interface {
	Greeting(ctx context.Context) (string, error)
}

// MessageRepository хранит тестовые строки (ЛР2).
type MessageRepository interface {
	Create(ctx context.Context, text string) (domain.Message, error)
	List(ctx context.Context, limit int) ([]domain.Message, error)
}

// UserRepository хранит пользователей (ЛР3).
type UserRepository interface {
	Create(ctx context.Context, login, passwordHash string) (domain.User, error)
	GetByLogin(ctx context.Context, login string) (domain.User, error)
}

// OrderRepository хранит заявки на полёт (ЛР4).
type OrderRepository interface {
	Create(ctx context.Context, o domain.Order) (domain.Order, error)
	ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error)
	GetByID(ctx context.Context, id, userID int64) (domain.Order, error)
	UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus, reason string) error
}

// TokenGenerator выпускает сессионные JWT-токены (ЛР3).
type TokenGenerator interface {
	Generate(userID int64) (token string, expiresAt time.Time, err error)
}

// EventPublisher отправляет события в брокер сообщений.
type EventPublisher interface {
	PublishOrderCreated(ctx context.Context, event contracts.OrderCreated) error
}
