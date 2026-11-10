package handler

import (
	"net/http"
	"strconv"

	"cosmoport/internal/api/middleware"
)

type createOrderRequest struct {
	Destination string `json:"destination"`
	Passengers  int    `json:"passengers"`
}

// destinations — GET /destinations возвращает каталог направлений (без авторизации).
func (h *Handler) destinations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.orders.Destinations())
}

// createOrder — ЛР4: POST /orders создаёт заявку на полёт для текущего пользователя.
// ID пользователя берётся из JWT-токена (его положил middleware Auth, ЛР5).
func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createOrderRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body, expected {\"destination\": \"moon\", \"passengers\": 1}")
		return
	}
	order, err := h.orders.Create(r.Context(), userID, req.Destination, req.Passengers)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

// listOrders — ЛР4: GET /orders возвращает все заявки текущего пользователя и их статусы.
// Поддерживает пагинацию: ?limit=50&offset=0.
func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	orders, err := h.orders.List(r.Context(), userID, limit, offset)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

// getOrder — GET /orders/{id} возвращает одну заявку (только свою).
func (h *Handler) getOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	orderID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orderID <= 0 {
		writeError(w, http.StatusBadRequest, "order id must be a positive integer")
		return
	}
	order, err := h.orders.Get(r.Context(), userID, orderID)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
