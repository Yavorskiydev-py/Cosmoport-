package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutesServeUI(t *testing.T) {
	mux := http.NewServeMux()
	Routes(mux, Links{Grafana: "http://grafana"})

	cases := map[string]string{
		"/":                 "Cosmoport",
		"/static/app.js":    "refreshOrders",
		"/static/style.css": ".track",
		"/ui/config":        `"grafana":"http://grafana"`,
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s: code=%d, body does not contain %q", path, rec.Code, want)
		}
	}
}
