// Команда api — основной сервис "Космопорт": регистрация, авторизация и заявки на полёты.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"cosmoport/internal/api/app"
	"cosmoport/internal/logger"
)

func main() {
	log := logger.New("api")

	// ctx отменяется при получении SIGINT (Ctrl+C) или SIGTERM (docker stop).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, app.LoadConfig(), log); err != nil {
		log.Error("service stopped with error", "error", err)
		os.Exit(1)
	}
	log.Info("service stopped gracefully")
}
