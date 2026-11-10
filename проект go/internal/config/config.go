// Package config содержит хелперы для чтения конфигурации из переменных окружения.
package config

import (
	"os"
	"strconv"
	"time"
)

// String возвращает значение переменной окружения или значение по умолчанию.
func String(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// Int возвращает целочисленное значение переменной окружения или значение по умолчанию.
func Int(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// Float возвращает дробное значение переменной окружения или значение по умолчанию.
func Float(key string, def float64) float64 {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// Duration возвращает длительность (например "5s", "2m") или значение по умолчанию.
func Duration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
