// Package app собирает основной сервис из слоёв (dependency injection) и управляет
// его жизненным циклом, включая graceful shutdown.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cosmoport/internal/api/auth"
	"cosmoport/internal/api/handler"
	"cosmoport/internal/api/messaging"
	"cosmoport/internal/api/metrics"
	"cosmoport/internal/api/middleware"
	"cosmoport/internal/api/repository/memory"
	"cosmoport/internal/api/repository/postgres"
	"cosmoport/internal/api/service"
	"cosmoport/internal/api/web"
	"cosmoport/internal/broker"
	"cosmoport/internal/config"
	"cosmoport/internal/contracts"
)

// Config — настройки основного сервиса (читаются из переменных окружения).
type Config struct {
	HTTPAddr        string
	DatabaseURL     string
	DBMaxConns      int
	RabbitURL       string
	JWTSecret       string
	JWTTTL          time.Duration
	BcryptCost      int
	ShutdownTimeout time.Duration
	Links           web.Links
}

// LoadConfig читает конфигурацию.
func LoadConfig() Config {
	return Config{
		HTTPAddr:        config.String("HTTP_ADDR", ":8080"),
		DatabaseURL:     config.String("DATABASE_URL", "postgres://cosmoport:cosmoport@localhost:25432/cosmoport?sslmode=disable"),
		DBMaxConns:      config.Int("DB_MAX_CONNS", 20),
		RabbitURL:       config.String("RABBITMQ_URL", "amqp://cosmoport:cosmoport@localhost:27672/"),
		JWTSecret:       config.String("JWT_SECRET", "change-me-in-production"),
		JWTTTL:          config.Duration("JWT_TTL", 24*time.Hour),
		BcryptCost:      config.Int("BCRYPT_COST", 10),
		ShutdownTimeout: config.Duration("SHUTDOWN_TIMEOUT", 15*time.Second),
		Links: web.Links{
			Grafana:    config.String("GRAFANA_PUBLIC_URL", "http://localhost:23000"),
			Prometheus: config.String("PROMETHEUS_PUBLIC_URL", "http://localhost:29090"),
			RabbitMQ:   config.String("RABBITMQ_PUBLIC_URL", "http://localhost:27673"),
		},
	}
}

// Run запускает сервис и блокируется, пока ctx не будет отменён (сигнал SIGINT/SIGTERM).
// Порядок остановки: HTTP-сервер перестаёт принимать запросы и дожидается текущих,
// затем останавливается потребитель RabbitMQ, затем закрываются брокер и пул БД.
func Run(ctx context.Context, cfg Config, log *slog.Logger) error {
	// --- инфраструктура ---
	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL, int32(cfg.DBMaxConns), log)
	if err != nil {
		return err
	}
	defer func() {
		pool.Close()
		log.Info("postgres pool closed")
	}()

	if err := postgres.Migrate(ctx, pool, log); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	mq, err := broker.Connect(ctx, cfg.RabbitURL, log)
	if err != nil {
		return err
	}
	defer func() {
		_ = mq.Close()
		log.Info("rabbitmq connection closed")
	}()
	if err := mq.DeclareQueues(contracts.QueueOrderCreated, contracts.QueueOrderStatus); err != nil {
		return err
	}

	// --- слой repository ---
	greetingRepo := memory.NewGreetingRepository()
	messageRepo := postgres.NewMessageRepository(pool)
	userRepo := postgres.NewUserRepository(pool)
	orderRepo := postgres.NewOrderRepository(pool)

	// --- слой service ---
	jwtManager := auth.NewJWTManager(cfg.JWTSecret, cfg.JWTTTL)
	greetingSvc := service.NewGreetingService(greetingRepo)
	messageSvc := service.NewMessageService(messageRepo)
	authSvc := service.NewAuthService(userRepo, jwtManager, cfg.BcryptCost)
	orderSvc := service.NewOrderService(orderRepo, messaging.NewPublisher(mq), log)

	// --- метрики ---
	metrics.RegisterDBPool(pool)
	metrics.RegisterOrdersByStatus(orderRepo, log)

	// --- слой handler ---
	mux := http.NewServeMux()
	handler.New(greetingSvc, messageSvc, authSvc, orderSvc, log).Routes(mux, middleware.Auth(jwtManager))
	web.Routes(mux, cfg.Links)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		hctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(hctx); err != nil {
			http.Error(w, `{"status":"db unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           middleware.Chain(mux, middleware.Recover(log), middleware.RequestID, middleware.Observe(log)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// --- запуск ---
	consumerCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	consumerDone := make(chan error, 1)
	go func() {
		consumerDone <- messaging.NewStatusConsumer(mq, orderSvc, log).Run(consumerCtx)
	}()

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	var runErr error
	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-serverErr:
		runErr = fmt.Errorf("http server: %w", err)
	case err := <-consumerDone:
		runErr = fmt.Errorf("status consumer: %w", err)
		consumerDone <- nil
	}

	// --- graceful shutdown ---
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http server shutdown", "error", err)
	} else {
		log.Info("http server stopped, all in-flight requests completed")
	}

	stopConsumer()
	select {
	case <-consumerDone:
	case <-shutdownCtx.Done():
		log.Warn("status consumer did not stop in time")
	}
	return runErr
}
