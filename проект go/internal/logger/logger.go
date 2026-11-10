// Package logger создаёт структурированный JSON-логгер на базе log/slog.
// JSON-логи собирает Promtail и отправляет в Loki, откуда их показывает Grafana.
package logger

import (
	"log/slog"
	"os"
	"strings"
)

// New создаёт логгер, у которого каждая запись помечена именем сервиса.
// Уровень логирования задаётся переменной окружения LOG_LEVEL (debug, info, warn, error).
func New(service string) *slog.Logger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLevel(os.Getenv("LOG_LEVEL"))})
	return slog.New(h).With("service", service)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
