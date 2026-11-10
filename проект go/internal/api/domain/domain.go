// Package domain содержит бизнес-сущности и ошибки предметной области "Космопорта".
// Пакет не зависит ни от HTTP, ни от БД — его используют все три слоя.
package domain

import (
	"errors"
	"time"
)

// Ошибки предметной области. Слой handler превращает их в HTTP-коды.
var (
	ErrNotFound           = errors.New("not found")
	ErrAlreadyExists      = errors.New("already exists")
	ErrInvalidCredentials = errors.New("invalid login or password")
	ErrValidation         = errors.New("validation error")
	ErrUnauthorized       = errors.New("unauthorized")
)

// Message — запись тестовой таблицы для ЛР2 (/dbtest).
type Message struct {
	ID        int64     `json:"id"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

// User — космический путешественник (пользователь сервиса).
type User struct {
	ID           int64     `json:"id"`
	Login        string    `json:"login"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Destination — направление полёта из каталога.
type Destination struct {
	Code         string `json:"code"`
	Name         string `json:"name"`
	DistanceKm   int64  `json:"distance_km"`
	FlightDays   int    `json:"flight_days"`
	PricePerSeat int64  `json:"price_per_seat"`
}

// OrderStatus — статус заявки на полёт.
type OrderStatus string

// Жизненный цикл заявки:
//
//	created -> processing -> approved -> launched -> completed
//	                      \-> rejected
const (
	StatusCreated    OrderStatus = "created"    // заявка принята космопортом
	StatusProcessing OrderStatus = "processing" // ЦУП проверяет экипаж и готовит ракету
	StatusApproved   OrderStatus = "approved"   // полёт одобрен
	StatusRejected   OrderStatus = "rejected"   // полёт отклонён
	StatusLaunched   OrderStatus = "launched"   // ракета стартовала
	StatusCompleted  OrderStatus = "completed"  // экипаж успешно прибыл
)

// Valid сообщает, является ли статус известным.
func (s OrderStatus) Valid() bool {
	switch s {
	case StatusCreated, StatusProcessing, StatusApproved, StatusRejected, StatusLaunched, StatusCompleted:
		return true
	}
	return false
}

// Final сообщает, является ли статус конечным (после него статус больше не меняется).
func (s OrderStatus) Final() bool {
	return s == StatusRejected || s == StatusCompleted
}

// Order — заявка на космический полёт.
type Order struct {
	ID           int64       `json:"id"`
	UserID       int64       `json:"user_id"`
	Destination  string      `json:"destination"`
	Passengers   int         `json:"passengers"`
	Price        int64       `json:"price"`
	Status       OrderStatus `json:"status"`
	StatusReason string      `json:"status_reason,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}
