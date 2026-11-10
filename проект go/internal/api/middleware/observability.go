package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"cosmoport/internal/api/metrics"
)

// statusRecorder запоминает код ответа, чтобы его можно было залогировать и посчитать.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// Chain применяет middleware в порядке перечисления (первый — самый внешний).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// RequestID присваивает каждому запросу идентификатор (заголовок X-Request-ID).
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			b := make([]byte, 8)
			_, _ = rand.Read(b)
			id = hex.EncodeToString(b)
			r.Header.Set("X-Request-ID", id)
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}

// Observe логирует каждый запрос и записывает метрики Prometheus.
// Маршрут берётся из r.Pattern (например "GET /orders/{id}"), поэтому ID заявок
// не раздувают количество меток.
func Observe(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			metrics.HTTPInFlight.Inc()
			defer metrics.HTTPInFlight.Dec()

			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)
			if rec.status == 0 {
				rec.status = http.StatusOK
			}

			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			elapsed := time.Since(start)
			metrics.HTTPRequests.WithLabelValues(route, r.Method, strconv.Itoa(rec.status)).Inc()
			metrics.HTTPDuration.WithLabelValues(route, r.Method).Observe(elapsed.Seconds())

			if route == "GET /metrics" || route == "GET /health" {
				return
			}
			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
			case rec.status >= 400:
				level = slog.LevelWarn
			}
			log.Log(r.Context(), level, "http request",
				"request_id", r.Header.Get("X-Request-ID"),
				"method", r.Method,
				"path", r.URL.Path,
				"route", route,
				"status", rec.status,
				"bytes", rec.bytes,
				"duration_ms", float64(elapsed.Microseconds())/1000,
			)
		})
	}
}

// Recover перехватывает панику в хэндлере и возвращает 500 вместо падения сервера.
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if p := recover(); p != nil {
					log.Error("panic in handler", "panic", p, "stack", string(debug.Stack()))
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
