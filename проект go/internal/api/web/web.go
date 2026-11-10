// Package web отдаёт веб-интерфейс "Космопорта" — одностраничное приложение на HTML/CSS/JS.
// Файлы встроены в бинарник через go:embed, поэтому отдельный веб-сервер не нужен.
// Интерфейс работает с тем же REST API, что и curl: /auth/*, /orders, /destinations, /test, /dbtest.
package web

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// Links — адреса инструментов мониторинга, которые показывает интерфейс.
type Links struct {
	Grafana    string `json:"grafana"`
	Prometheus string `json:"prometheus"`
	RabbitMQ   string `json:"rabbitmq"`
}

// Routes регистрирует страницу интерфейса, статические файлы и конфиг для фронтенда.
func Routes(mux *http.ServeMux, links Links) {
	static, _ := fs.Sub(staticFS, "static")

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, static, "index.html")
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /ui/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(links)
	})
}
