// Package metrics описывает метрики Prometheus основного сервиса.
// Все метрики доступны по адресу GET /metrics и собираются Prometheus каждые 5 секунд.
package metrics

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "cosmoport"

var (
	// HTTPRequests — количество HTTP-запросов по маршруту, методу и коду ответа.
	HTTPRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "Total number of HTTP requests.",
	}, []string{"route", "method", "code"})

	// HTTPDuration — время обработки HTTP-запросов.
	HTTPDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request latency.",
		Buckets:   []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
	}, []string{"route", "method"})

	// HTTPInFlight — сколько запросов обрабатывается прямо сейчас.
	HTTPInFlight = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "http_requests_in_flight",
		Help:      "Number of HTTP requests being processed.",
	})

	// UsersRegistered — количество успешных регистраций.
	UsersRegistered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "users_registered_total",
		Help:      "Total number of registered users.",
	})

	// Logins — попытки входа (result = success | failure).
	Logins = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "logins_total",
		Help:      "Login attempts by result.",
	}, []string{"result"})

	// OrdersCreated — созданные заявки по направлениям.
	OrdersCreated = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "orders_created_total",
		Help:      "Total number of created flight orders.",
	}, []string{"destination"})

	// OrderStatusUpdates — применённые изменения статусов, пришедшие из mission-control.
	OrderStatusUpdates = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "order_status_updates_total",
		Help:      "Order status updates applied from mission-control.",
	}, []string{"status"})

	// EventsPublished — публикации событий в RabbitMQ (result = success | failure).
	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_published_total",
		Help:      "Events published to RabbitMQ.",
	}, []string{"queue", "result"})
)

// StatusCounter умеет считать заявки по статусам (реализуется репозиторием заявок).
type StatusCounter interface {
	CountByStatus(ctx context.Context) (map[string]int64, error)
}

// ordersByStatusCollector при каждом опросе Prometheus считает текущее число заявок в каждом статусе.
type ordersByStatusCollector struct {
	counter StatusCounter
	desc    *prometheus.Desc
	log     *slog.Logger
}

func (c *ordersByStatusCollector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

func (c *ordersByStatusCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	counts, err := c.counter.CountByStatus(ctx)
	if err != nil {
		c.log.Warn("collect orders by status", "error", err)
		return
	}
	for status, n := range counts {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(n), status)
	}
}

// RegisterOrdersByStatus регистрирует метрику cosmoport_orders_by_status.
func RegisterOrdersByStatus(counter StatusCounter, log *slog.Logger) {
	prometheus.MustRegister(&ordersByStatusCollector{
		counter: counter,
		log:     log,
		desc: prometheus.NewDesc(namespace+"_orders_by_status",
			"Current number of orders in each status.", []string{"status"}, nil),
	})
}

// RegisterDBPool регистрирует метрики пула соединений с PostgreSQL.
func RegisterDBPool(pool *pgxpool.Pool) {
	gauge := func(name, help string, f func(*pgxpool.Stat) float64) {
		prometheus.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "db_pool", Name: name, Help: help,
		}, func() float64 { return f(pool.Stat()) }))
	}
	gauge("acquired_connections", "Connections currently in use.", func(s *pgxpool.Stat) float64 { return float64(s.AcquiredConns()) })
	gauge("idle_connections", "Idle connections.", func(s *pgxpool.Stat) float64 { return float64(s.IdleConns()) })
	gauge("total_connections", "Total connections.", func(s *pgxpool.Stat) float64 { return float64(s.TotalConns()) })
	gauge("max_connections", "Maximum pool size.", func(s *pgxpool.Stat) float64 { return float64(s.MaxConns()) })
}
