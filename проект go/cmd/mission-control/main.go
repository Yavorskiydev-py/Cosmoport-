// Команда mission-control — сервис обработки заявок "Центр управления полётами".
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cosmoport/internal/broker"
	"cosmoport/internal/config"
	"cosmoport/internal/contracts"
	"cosmoport/internal/logger"
	"cosmoport/internal/mission"
)

func main() {
	log := logger.New("mission-control")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mq, err := broker.Connect(ctx, config.String("RABBITMQ_URL", "amqp://cosmoport:cosmoport@localhost:27672/"), log)
	if err != nil {
		log.Error("connect to rabbitmq", "error", err)
		os.Exit(1)
	}
	defer mq.Close()
	if err := mq.DeclareQueues(contracts.QueueOrderCreated, contracts.QueueOrderStatus); err != nil {
		log.Error("declare queues", "error", err)
		os.Exit(1)
	}

	// HTTP только для /metrics и /health.
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	srv := &http.Server{Addr: config.String("HTTP_ADDR", ":8081"), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("metrics server started", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server", "error", err)
			stop()
		}
	}()

	processor := mission.NewProcessor(mq, mq, mission.Config{
		Workers:       config.Int("WORKERS", 50),
		MinStageDelay: config.Duration("MIN_STAGE_DELAY", 1*time.Second),
		MaxStageDelay: config.Duration("MAX_STAGE_DELAY", 3*time.Second),
		RejectRate:    config.Float("REJECT_RATE", 0.1),
	}, log)

	runErr := processor.Run(ctx)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)

	if runErr != nil {
		log.Error("mission control stopped with error", "error", runErr)
		os.Exit(1)
	}
	log.Info("mission control stopped gracefully")
}
