// Package auth выпускает и проверяет сессионные JWT-токены (ЛР3, ЛР5).
package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken — токен отсутствует, подделан или просрочен.
var ErrInvalidToken = errors.New("invalid token")

// JWTManager подписывает токены алгоритмом HS256 секретным ключом.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	issuer string
}

// NewJWTManager создаёт менеджер токенов. ttl — время жизни токена.
func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), ttl: ttl, issuer: "cosmoport"}
}

// Generate выпускает токен, в поле sub которого записан ID пользователя.
func (m *JWTManager) Generate(userID int64) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(m.ttl)
	claims := jwt.RegisteredClaims{
		Subject:   strconv.FormatInt(userID, 10),
		Issuer:    m.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(expiresAt),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return token, expiresAt, nil
}

// Parse проверяет подпись и срок действия токена и возвращает ID пользователя.
func (m *JWTManager) Parse(tokenString string) (int64, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(tokenString, claims,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(m.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return 0, fmt.Errorf("%w: bad subject", ErrInvalidToken)
	}
	return userID, nil
}
