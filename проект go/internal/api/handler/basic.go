package handler

import (
	"io"
	"net/http"
)

const maxTextBody = 4 << 10 // 4 КБ

// test — ЛР1: GET /test возвращает строку "Hello!".
// Цепочка вызовов: handler.test -> GreetingService.Hello -> GreetingRepository.Greeting.
func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	msg, err := h.greeting.Hello(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, msg)
}

// createMessage — ЛР2: POST /dbtest записывает строку из тела запроса в БД.
func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTextBody))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "body is too large (max 4 KB)")
		return
	}
	msg, err := h.messages.Save(r.Context(), string(body))
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, msg)
}

// listMessages — GET /dbtest показывает последние записанные строки (для проверки чтения из БД).
func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	msgs, err := h.messages.Latest(r.Context())
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, msgs)
}
