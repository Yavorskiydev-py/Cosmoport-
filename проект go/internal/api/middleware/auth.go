// Package middleware содержит HTTP-обёртки: авторизацию по JWT (ЛР5), логирование,
// метрики, request-id и перехват паник.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type ctxKey int

const userIDKey ctxKey = iota

// TokenParser проверяет токен и возвращает ID пользователя.
type TokenParser interface {
	Parse(token string) (int64, error)
}

// Auth — middleware ЛР5: принимает сессионный токен из заголовка
// "Authorization: Bearer <token>", проверяет его и кладёт ID пользователя в контекст запроса.
// Хэндлеры получают ID через UserIDFromContext.
func Auth(parser TokenParser) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			scheme, token, ok := strings.Cut(header, " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
				unauthorized(w, "missing or malformed Authorization header, expected: Bearer <token>")
				return
			}

			userID, err := parser.Parse(strings.TrimSpace(token))
			if err != nil {
				unauthorized(w, "invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserIDFromContext достаёт ID пользователя, которого положил middleware Auth.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey).(int64)
	return id, ok
}

// WithUserID кладёт ID пользователя в контекст (используется в тестах).
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("WWW-Authenticate", `Bearer realm="cosmoport"`)
	w.WriteHeader(http.StatusUnauthorized)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{"error": msg})
}
