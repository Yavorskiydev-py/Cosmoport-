// Команда loadgen — нагрузочный сервис для демонстрации работоспособности системы.
//
// Пример: go run ./cmd/loadgen -users 50 -duration 3m
package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"cosmoport/internal/config"
	"cosmoport/internal/loadgen"
	"cosmoport/internal/logger"
)

func main() {
	log := logger.New("loadgen")

	var cfg loadgen.Config
	flag.StringVar(&cfg.TargetURL, "target", config.String("TARGET_URL", "http://localhost:28080"), "base URL of the api service")
	flag.IntVar(&cfg.Users, "users", config.Int("USERS", 30), "number of virtual users")
	flag.DurationVar(&cfg.Duration, "duration", config.Duration("DURATION", 5*time.Minute), "test duration (0 = until Ctrl+C)")
	flag.DurationVar(&cfg.RampUp, "ramp-up", config.Duration("RAMP_UP", 30*time.Second), "time to start all virtual users")
	flag.DurationVar(&cfg.ThinkMin, "think-min", config.Duration("THINK_MIN", 100*time.Millisecond), "min pause between actions")
	flag.DurationVar(&cfg.ThinkMax, "think-max", config.Duration("THINK_MAX", 500*time.Millisecond), "max pause between actions")
	metricsAddr := flag.String("metrics-addr", config.String("METRICS_ADDR", ":9100"), "address for /metrics")
	flag.Parse()

	if cfg.Users < 1 {
		cfg.Users = 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	srv := &http.Server{Addr: *metricsAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server", "error", err)
		}
	}()

	loadgen.NewRunner(cfg, log).Run(ctx)

	// Даём Prometheus время забрать финальные значения метрик.
	select {
	case <-ctx.Done():
	case <-time.After(config.Duration("LINGER", 15*time.Second)):
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
