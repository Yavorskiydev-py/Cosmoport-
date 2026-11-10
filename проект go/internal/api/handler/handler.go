// Package handler — транспортный слой (первый слой чистой архитектуры).
//
// Хэндлеры только разбирают HTTP-запрос, вызывают слой service через интерфейсы
// и превращают результат в HTTP-ответ. Бизнес-логики здесь нет.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"cosmoport/internal/api/domain"
)

// GreetingService — бизнес-логика /test.
type GreetingService interface {
	Hello(ctx context.Context) (string, error)
}

// MessageService — бизнес-логика /dbtest.
type MessageService interface {
	Save(ctx context.Context, text string) (domain.Message, error)
	Latest(ctx context.Context) ([]domain.Message, error)
}

// AuthService — регистрация и вход.
type AuthService interface {
	Register(ctx context.Context, login, password string) (domain.User, error)
	Login(ctx context.Context, login, password string) (string, time.Time, error)
}

// OrderService — заявки на полёт.
type OrderService interface {
	Destinations() []domain.Destination
	Create(ctx context.Context, userID int64, destination string, passengers int) (domain.Order, error)
	List(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error)
	Get(ctx context.Context, userID, orderID int64) (domain.Order, error)
}

// Handler объединяет все HTTP-хэндлеры сервиса.
type Handler struct {
	greeting GreetingService
	messages MessageService
	auth     AuthService
	orders   OrderService
	log      *slog.Logger
}

// New создаёт Handler.
func New(greeting GreetingService, messages MessageService, auth AuthService, orders OrderService, log *slog.Logger) *Handler {
	return &Handler{greeting: greeting, messages: messages, auth: auth, orders: orders, log: log}
}

// Routes регистрирует маршруты. authMW — middleware проверки JWT для защищённых маршрутов.
func (h *Handler) Routes(mux *http.ServeMux, authMW func(http.Handler) http.Handler) {
	// ЛР1
	mux.HandleFunc("GET /test", h.test)
	// ЛР2
	mux.HandleFunc("POST /dbtest", h.createMessage)
	mux.HandleFunc("GET /dbtest", h.listMessages)
	// ЛР3
	mux.HandleFunc("POST /auth/register", h.register)
	mux.HandleFunc("POST /auth/login", h.login)
	// ЛР4 + ЛР5 (защищены middleware авторизации)
	mux.HandleFunc("GET /destinations", h.destinations)
	mux.Handle("POST /orders", authMW(http.HandlerFunc(h.createOrder)))
	mux.Handle("GET /orders", authMW(http.HandlerFunc(h.listOrders)))
	mux.Handle("GET /orders/{id}", authMW(http.HandlerFunc(h.getOrder)))
}

const maxJSONBody = 1 << 20 // 1 МБ

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// decodeJSON читает JSON-тело запроса, запрещая лишние поля и слишком большие тела.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

// handleError превращает ошибки предметной области в HTTP-коды.
func (h *Handler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, domain.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, domain.ErrAlreadyExists):
		writeError(w, http.StatusConflict, "already exists")
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		writeError(w, http.StatusServiceUnavailable, "request cancelled")
	default:
		h.log.Error("internal error", "path", r.URL.Path, "request_id", r.Header.Get("X-Request-ID"), "error", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
