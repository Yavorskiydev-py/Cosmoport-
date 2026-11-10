package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"

	"cosmoport/internal/api/domain"
	"cosmoport/internal/api/metrics"
)

var loginRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,32}$`)

const (
	minPasswordLen = 6
	maxPasswordLen = 72 // ограничение bcrypt
)

// AuthService — регистрация и авторизация пользователей (ЛР3).
type AuthService struct {
	users      UserRepository
	tokens     TokenGenerator
	bcryptCost int
	dummyHash  []byte
}

// NewAuthService создаёт сервис авторизации. bcryptCost — "стоимость" хэширования пароля.
func NewAuthService(users UserRepository, tokens TokenGenerator, bcryptCost int) *AuthService {
	if bcryptCost < bcrypt.MinCost || bcryptCost > bcrypt.MaxCost {
		bcryptCost = bcrypt.DefaultCost
	}
	// Хэш-заглушка: сравниваем с ним, даже если пользователя нет, чтобы по времени ответа
	// нельзя было понять, существует ли логин.
	dummy, _ := bcrypt.GenerateFromPassword([]byte("cosmoport-dummy-password"), bcryptCost)
	return &AuthService{users: users, tokens: tokens, bcryptCost: bcryptCost, dummyHash: dummy}
}

// Register создаёт нового пользователя. Пароль хранится в БД только в виде bcrypt-хэша.
func (s *AuthService) Register(ctx context.Context, login, password string) (domain.User, error) {
	if !loginRe.MatchString(login) {
		return domain.User{}, fmt.Errorf("%w: login must be 3-32 characters: letters, digits, '_', '.', '-'", domain.ErrValidation)
	}
	if len(password) < minPasswordLen || len(password) > maxPasswordLen {
		return domain.User{}, fmt.Errorf("%w: password must be %d-%d characters long", domain.ErrValidation, minPasswordLen, maxPasswordLen)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	user, err := s.users.Create(ctx, login, string(hash))
	if err != nil {
		return domain.User{}, err
	}
	metrics.UsersRegistered.Inc()
	return user, nil
}

// Login проверяет логин и пароль и выдаёт сессионный JWT-токен.
func (s *AuthService) Login(ctx context.Context, login, password string) (string, time.Time, error) {
	user, err := s.users.GetByLogin(ctx, login)
	if errors.Is(err, domain.ErrNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		metrics.Logins.WithLabelValues("failure").Inc()
		return "", time.Time{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		metrics.Logins.WithLabelValues("failure").Inc()
		return "", time.Time{}, domain.ErrInvalidCredentials
	}

	token, expiresAt, err := s.tokens.Generate(user.ID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("generate token: %w", err)
	}
	metrics.Logins.WithLabelValues("success").Inc()
	return token, expiresAt, nil
}
