package handler

import (
	"net/http"
	"time"
)

type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
}

// register — ЛР3: POST /auth/register создаёт пользователя в БД.
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body, expected {\"login\": \"...\", \"password\": \"...\"}")
		return
	}
	user, err := h.auth.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// login — ЛР3: POST /auth/login проверяет пароль и выдаёт сессионный JWT-токен.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body, expected {\"login\": \"...\", \"password\": \"...\"}")
		return
	}
	token, expiresAt, err := h.auth.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		h.handleError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, loginResponse{Token: token, TokenType: "Bearer", ExpiresAt: expiresAt})
}
