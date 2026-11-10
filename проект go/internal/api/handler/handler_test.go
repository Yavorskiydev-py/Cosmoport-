package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmoport/internal/api/auth"
	"cosmoport/internal/api/domain"
	"cosmoport/internal/api/middleware"
	"cosmoport/internal/api/repository/memory"
	"cosmoport/internal/api/service"
)

type stubMessages struct{ saved []string }

func (s *stubMessages) Save(_ context.Context, text string) (domain.Message, error) {
	if strings.TrimSpace(text) == "" {
		return domain.Message{}, domain.ErrValidation
	}
	s.saved = append(s.saved, text)
	return domain.Message{ID: int64(len(s.saved)), Text: text}, nil
}
func (s *stubMessages) Latest(context.Context) ([]domain.Message, error) { return nil, nil }

type stubAuth struct{}

func (stubAuth) Register(_ context.Context, login, _ string) (domain.User, error) {
	if login == "taken" {
		return domain.User{}, domain.ErrAlreadyExists
	}
	return domain.User{ID: 1, Login: login}, nil
}
func (stubAuth) Login(context.Context, string, string) (string, time.Time, error) {
	return "", time.Time{}, domain.ErrInvalidCredentials
}

type stubOrders struct{ lastUserID int64 }

func (s *stubOrders) Destinations() []domain.Destination { return nil }
func (s *stubOrders) Create(_ context.Context, userID int64, dest string, n int) (domain.Order, error) {
	s.lastUserID = userID
	return domain.Order{ID: 1, UserID: userID, Destination: dest, Passengers: n, Status: domain.StatusCreated}, nil
}
func (s *stubOrders) List(_ context.Context, userID int64, _, _ int) ([]domain.Order, error) {
	s.lastUserID = userID
	return []domain.Order{}, nil
}
func (s *stubOrders) Get(context.Context, int64, int64) (domain.Order, error) {
	return domain.Order{}, domain.ErrNotFound
}

func newTestServer(t *testing.T) (http.Handler, *auth.JWTManager, *stubOrders, *stubMessages) {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwt := auth.NewJWTManager("test-secret", time.Hour)
	orders := &stubOrders{}
	messages := &stubMessages{}
	// Для /test используем настоящие слои service и repository — проверяем всю цепочку.
	greeting := service.NewGreetingService(memory.NewGreetingRepository())

	mux := http.NewServeMux()
	New(greeting, messages, stubAuth{}, orders, log).Routes(mux, middleware.Auth(jwt))
	return mux, jwt, orders, messages
}

func do(h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestTestEndpointReturnsHello(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	rec := do(h, http.MethodGet, "/test", "", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "Hello!" {
		t.Fatalf("GET /test: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestDBTestSavesBody(t *testing.T) {
	h, _, _, messages := newTestServer(t)
	rec := do(h, http.MethodPost, "/dbtest", "Поехали!", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /dbtest: code=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(messages.saved) != 1 || messages.saved[0] != "Поехали!" {
		t.Fatalf("body must be passed to the service, got %v", messages.saved)
	}
	if rec := do(h, http.MethodPost, "/dbtest", "  ", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("empty body: want 400, got %d", rec.Code)
	}
}

func TestRegisterErrors(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	if rec := do(h, http.MethodPost, "/auth/register", "{bad json", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: want 400, got %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/auth/register", `{"login":"taken","password":"secret1"}`, ""); rec.Code != http.StatusConflict {
		t.Fatalf("taken login: want 409, got %d", rec.Code)
	}
	if rec := do(h, http.MethodPost, "/auth/login", `{"login":"x","password":"y"}`, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad credentials: want 401, got %d", rec.Code)
	}
}

func TestOrdersRequireToken(t *testing.T) {
	h, _, _, _ := newTestServer(t)
	for _, token := range []string{"", "broken-token"} {
		if rec := do(h, http.MethodGet, "/orders", "", token); rec.Code != http.StatusUnauthorized {
			t.Fatalf("token %q: want 401, got %d", token, rec.Code)
		}
	}
}

func TestMiddlewarePassesUserIDFromToken(t *testing.T) {
	h, jwt, orders, _ := newTestServer(t)
	token, _, _ := jwt.Generate(77)

	rec := do(h, http.MethodPost, "/orders", `{"destination":"mars","passengers":2}`, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var order domain.Order
	_ = json.Unmarshal(rec.Body.Bytes(), &order)
	if orders.lastUserID != 77 || order.UserID != 77 {
		t.Fatalf("user id from token must reach the handler, got %d", orders.lastUserID)
	}

	if rec := do(h, http.MethodGet, "/orders", "", token); rec.Code != http.StatusOK {
		t.Fatalf("list orders: want 200, got %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/orders/abc", "", token); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: want 400, got %d", rec.Code)
	}
	if rec := do(h, http.MethodGet, "/orders/5", "", token); rec.Code != http.StatusNotFound {
		t.Fatalf("foreign order: want 404, got %d", rec.Code)
	}
}
