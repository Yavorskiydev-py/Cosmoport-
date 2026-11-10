package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cosmoport/internal/api/domain"
	"cosmoport/internal/contracts"
)

// --- фейковые реализации интерфейсов: сервисы тестируются без БД и брокера ---

type fakeUsers struct {
	mu    sync.Mutex
	users map[string]domain.User
}

func newFakeUsers() *fakeUsers { return &fakeUsers{users: map[string]domain.User{}} }

func (f *fakeUsers) Create(_ context.Context, login, hash string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[login]; ok {
		return domain.User{}, domain.ErrAlreadyExists
	}
	u := domain.User{ID: int64(len(f.users) + 1), Login: login, PasswordHash: hash}
	f.users[login] = u
	return u, nil
}

func (f *fakeUsers) GetByLogin(_ context.Context, login string) (domain.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[login]
	if !ok {
		return domain.User{}, domain.ErrNotFound
	}
	return u, nil
}

type fakeTokens struct{}

func (fakeTokens) Generate(userID int64) (string, time.Time, error) {
	return "token-for-user", time.Now().Add(time.Hour), nil
}

type fakeOrders struct {
	created []domain.Order
	updated map[int64]domain.OrderStatus
}

func (f *fakeOrders) Create(_ context.Context, o domain.Order) (domain.Order, error) {
	o.ID = int64(len(f.created) + 1)
	f.created = append(f.created, o)
	return o, nil
}

func (f *fakeOrders) ListByUser(_ context.Context, userID int64, limit, _ int) ([]domain.Order, error) {
	var out []domain.Order
	for _, o := range f.created {
		if o.UserID == userID && len(out) < limit {
			out = append(out, o)
		}
	}
	return out, nil
}

func (f *fakeOrders) GetByID(_ context.Context, id, userID int64) (domain.Order, error) {
	for _, o := range f.created {
		if o.ID == id && o.UserID == userID {
			return o, nil
		}
	}
	return domain.Order{}, domain.ErrNotFound
}

func (f *fakeOrders) UpdateStatus(_ context.Context, id int64, status domain.OrderStatus, _ string) error {
	if f.updated == nil {
		f.updated = map[int64]domain.OrderStatus{}
	}
	f.updated[id] = status
	return nil
}

type fakePublisher struct {
	events []contracts.OrderCreated
	err    error
}

func (f *fakePublisher) PublishOrderCreated(_ context.Context, e contracts.OrderCreated) error {
	f.events = append(f.events, e)
	return f.err
}

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

// --- тесты ---

func TestAuthService_RegisterAndLogin(t *testing.T) {
	ctx := context.Background()
	svc := NewAuthService(newFakeUsers(), fakeTokens{}, bcrypt.MinCost)

	user, err := svc.Register(ctx, "gagarin", "poekhali")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.PasswordHash == "poekhali" {
		t.Fatal("password must be stored as a hash")
	}

	if _, err := svc.Register(ctx, "gagarin", "another1"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate login: want ErrAlreadyExists, got %v", err)
	}

	token, _, err := svc.Login(ctx, "gagarin", "poekhali")
	if err != nil || token == "" {
		t.Fatalf("login: token=%q err=%v", token, err)
	}

	if _, _, err := svc.Login(ctx, "gagarin", "wrong-password"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("wrong password: want ErrInvalidCredentials, got %v", err)
	}
	if _, _, err := svc.Login(ctx, "nobody", "whatever"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("unknown user: want ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthService_RegisterValidation(t *testing.T) {
	svc := NewAuthService(newFakeUsers(), fakeTokens{}, bcrypt.MinCost)
	cases := map[string][2]string{
		"short login":    {"ab", "password"},
		"bad characters": {"юрий гагарин", "password"},
		"short password": {"titov", "123"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Register(context.Background(), c[0], c[1]); !errors.Is(err, domain.ErrValidation) {
				t.Fatalf("want ErrValidation, got %v", err)
			}
		})
	}
}

func TestOrderService_Create(t *testing.T) {
	repo := &fakeOrders{}
	pub := &fakePublisher{}
	svc := NewOrderService(repo, pub, discard)

	order, err := svc.Create(context.Background(), 7, "moon", 2)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if order.Status != domain.StatusCreated || order.UserID != 7 {
		t.Fatalf("unexpected order: %+v", order)
	}
	if order.Price != 2*150_000 {
		t.Fatalf("price: want 300000, got %d", order.Price)
	}
	if len(pub.events) != 1 || pub.events[0].OrderID != order.ID {
		t.Fatalf("event for mission-control must be published, got %+v", pub.events)
	}
}

func TestOrderService_CreateValidation(t *testing.T) {
	svc := NewOrderService(&fakeOrders{}, &fakePublisher{}, discard)
	if _, err := svc.Create(context.Background(), 1, "pluto", 1); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("unknown destination: want ErrValidation, got %v", err)
	}
	if _, err := svc.Create(context.Background(), 1, "mars", 0); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("zero passengers: want ErrValidation, got %v", err)
	}
}

func TestOrderService_CreateSurvivesBrokerFailure(t *testing.T) {
	pub := &fakePublisher{err: errors.New("broker is down")}
	svc := NewOrderService(&fakeOrders{}, pub, discard)
	if _, err := svc.Create(context.Background(), 1, "iss", 1); err != nil {
		t.Fatalf("order is saved, broker error must not fail the request: %v", err)
	}
}

func TestOrderService_UsersSeeOnlyTheirOrders(t *testing.T) {
	ctx := context.Background()
	svc := NewOrderService(&fakeOrders{}, &fakePublisher{}, discard)
	o1, _ := svc.Create(ctx, 1, "moon", 1)
	_, _ = svc.Create(ctx, 2, "mars", 1)

	list, _ := svc.List(ctx, 1, 0, 0)
	if len(list) != 1 || list[0].ID != o1.ID {
		t.Fatalf("user 1 must see only own order, got %+v", list)
	}
	if _, err := svc.Get(ctx, 2, o1.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("user 2 must not see order of user 1, got %v", err)
	}
}

func TestOrderService_ApplyStatus(t *testing.T) {
	repo := &fakeOrders{}
	svc := NewOrderService(repo, &fakePublisher{}, discard)

	if err := svc.ApplyStatus(context.Background(), contracts.OrderStatusChanged{OrderID: 1, Status: "launched"}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if repo.updated[1] != domain.StatusLaunched {
		t.Fatalf("status was not saved: %+v", repo.updated)
	}
	if err := svc.ApplyStatus(context.Background(), contracts.OrderStatusChanged{OrderID: 1, Status: "teleported"}); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("unknown status: want ErrValidation, got %v", err)
	}
}
