// Package loadgen — нагрузочный сервис. Запускает N "виртуальных космонавтов", каждый из
// которых регистрируется, входит в систему и в цикле выполняет сценарий: смотрит каталог,
// создаёт заявки на полёт, проверяет их статусы. Свои метрики отдаёт в Prometheus.
package loadgen

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loadgen", Name: "requests_total",
		Help: "Requests sent by the load generator.",
	}, []string{"op", "code"})
	requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "loadgen", Name: "request_duration_seconds",
		Help:    "Latency observed by the load generator.",
		Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"op"})
	unexpected = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loadgen", Name: "unexpected_responses_total",
		Help: "Responses with an unexpected status code or transport errors.",
	}, []string{"op"})
	virtualUsers = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "loadgen", Name: "virtual_users",
		Help: "Currently active virtual users.",
	})
)

// Config — параметры нагрузки.
type Config struct {
	TargetURL string        // адрес основного сервиса
	Users     int           // число виртуальных пользователей
	Duration  time.Duration // длительность теста (0 — бесконечно, до Ctrl+C)
	RampUp    time.Duration // за сколько времени плавно запустить всех пользователей
	ThinkMin  time.Duration // минимальная пауза между действиями пользователя
	ThinkMax  time.Duration // максимальная пауза между действиями пользователя
}

var destinations = []string{"iss", "moon", "mars", "europa", "titan"}

// Runner выполняет нагрузочный тест.
type Runner struct {
	cfg    Config
	client *http.Client
	log    *slog.Logger
	runID  string

	total  atomic.Int64
	failed atomic.Int64
	orders atomic.Int64
}

// NewRunner создаёт Runner.
func NewRunner(cfg Config, log *slog.Logger) *Runner {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = cfg.Users * 2
	transport.MaxIdleConnsPerHost = cfg.Users * 2
	return &Runner{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second, Transport: transport},
		log:    log,
		runID:  strconv.FormatInt(time.Now().Unix(), 36),
	}
}

// Run запускает пользователей и ждёт окончания теста.
func (r *Runner) Run(ctx context.Context) {
	if r.cfg.Duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.cfg.Duration)
		defer cancel()
	}

	r.waitForTarget(ctx)
	r.log.Info("load test started", "target", r.cfg.TargetURL, "users", r.cfg.Users,
		"duration", r.cfg.Duration.String(), "ramp_up", r.cfg.RampUp.String())

	start := time.Now()
	var wg sync.WaitGroup
	step := time.Duration(0)
	if r.cfg.Users > 1 {
		step = r.cfg.RampUp / time.Duration(r.cfg.Users)
	}

launch:
	for i := 0; i < r.cfg.Users; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			virtualUsers.Inc()
			defer virtualUsers.Dec()
			r.virtualUser(ctx, n)
		}(i)
		if step > 0 {
			select {
			case <-ctx.Done():
				break launch
			case <-time.After(step):
			}
		}
	}
	wg.Wait()

	elapsed := time.Since(start)
	total := r.total.Load()
	r.log.Info("load test finished",
		"duration", elapsed.Round(time.Second).String(),
		"requests", total,
		"unexpected", r.failed.Load(),
		"orders_created", r.orders.Load(),
		"avg_rps", fmt.Sprintf("%.1f", float64(total)/elapsed.Seconds()),
	)
}

// waitForTarget ждёт, пока основной сервис ответит на /health.
func (r *Runner) waitForTarget(ctx context.Context) {
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.TargetURL+"/health", nil)
		resp, err := r.client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		r.log.Info("waiting for target", "url", r.cfg.TargetURL)
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// virtualUser — сценарий одного пользователя.
func (r *Runner) virtualUser(ctx context.Context, n int) {
	login := fmt.Sprintf("cosmonaut_%s_%d", r.runID, n)
	password := "star-" + login

	creds := map[string]string{"login": login, "password": password}
	if code, _, ok := r.call(ctx, "register", http.MethodPost, "/auth/register", "", creds, http.StatusCreated, http.StatusConflict); !ok && code == 0 {
		return
	}

	var token string
	for token == "" {
		code, body, ok := r.call(ctx, "login", http.MethodPost, "/auth/login", "", creds, http.StatusOK)
		if ok {
			var resp struct {
				Token string `json:"token"`
			}
			_ = json.Unmarshal(body, &resp)
			token = resp.Token
		} else if code == 0 && ctx.Err() != nil {
			return
		}
		if token == "" && !r.think(ctx) {
			return
		}
	}

	var myOrders []int64
	for r.think(ctx) {
		switch p := rand.IntN(100); {
		case p < 30: // создать заявку
			req := map[string]any{"destination": destinations[rand.IntN(len(destinations))], "passengers": 1 + rand.IntN(4)}
			if _, body, ok := r.call(ctx, "create_order", http.MethodPost, "/orders", token, req, http.StatusCreated); ok {
				var o struct {
					ID int64 `json:"id"`
				}
				if json.Unmarshal(body, &o) == nil && o.ID > 0 {
					myOrders = append(myOrders, o.ID)
					r.orders.Add(1)
				}
			}
		case p < 65: // список своих заявок со статусами
			r.call(ctx, "list_orders", http.MethodGet, "/orders?limit=20", token, nil, http.StatusOK)
		case p < 80: // статус одной заявки
			if len(myOrders) > 0 {
				id := myOrders[rand.IntN(len(myOrders))]
				r.call(ctx, "get_order", http.MethodGet, "/orders/"+strconv.FormatInt(id, 10), token, nil, http.StatusOK)
			}
		case p < 90:
			r.call(ctx, "destinations", http.MethodGet, "/destinations", "", nil, http.StatusOK)
		case p < 95:
			r.call(ctx, "test", http.MethodGet, "/test", "", nil, http.StatusOK)
		default: // намеренно неверный токен — сервер должен ответить 401
			r.call(ctx, "bad_token", http.MethodGet, "/orders", "invalid.token.value", nil, http.StatusUnauthorized)
		}
	}
}

// think — пауза между действиями. Возвращает false, если тест закончился.
func (r *Runner) think(ctx context.Context) bool {
	d := r.cfg.ThinkMin
	if spread := r.cfg.ThinkMax - r.cfg.ThinkMin; spread > 0 {
		d += rand.N(spread)
	}
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// call выполняет запрос и учитывает его в метриках. ok == true, если код ответа ожидаемый.
func (r *Runner) call(ctx context.Context, op, method, path, token string, body any, expected ...int) (int, []byte, bool) {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.cfg.TargetURL+path, reader)
	if err != nil {
		return 0, nil, false
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	start := time.Now()
	resp, err := r.client.Do(req)
	requestDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, false // тест закончился — это не ошибка сервера
		}
		r.total.Add(1)
		r.failed.Add(1)
		requestsTotal.WithLabelValues(op, "error").Inc()
		unexpected.WithLabelValues(op).Inc()
		r.log.Warn("request failed", "op", op, "error", err)
		return 0, nil, false
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	r.total.Add(1)
	requestsTotal.WithLabelValues(op, strconv.Itoa(resp.StatusCode)).Inc()
	for _, code := range expected {
		if resp.StatusCode == code {
			return resp.StatusCode, respBody, true
		}
	}
	r.failed.Add(1)
	unexpected.WithLabelValues(op).Inc()
	r.log.Warn("unexpected response", "op", op, "status", resp.StatusCode, "body", string(respBody))
	return resp.StatusCode, respBody, false
}
