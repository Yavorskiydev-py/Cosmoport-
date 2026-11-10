package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmoport/internal/api/domain"
)

const uniqueViolation = "23505"

// UserRepository работает с таблицей users (ЛР3).
type UserRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository создаёт репозиторий пользователей.
func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// Create сохраняет нового пользователя. Если логин занят — возвращает domain.ErrAlreadyExists.
func (r *UserRepository) Create(ctx context.Context, login, passwordHash string) (domain.User, error) {
	u := domain.User{Login: login, PasswordHash: passwordHash}
	err := r.pool.QueryRow(ctx,
		`INSERT INTO users (login, password_hash) VALUES ($1, $2) RETURNING id, created_at`,
		login, passwordHash,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return domain.User{}, domain.ErrAlreadyExists
		}
		return domain.User{}, fmt.Errorf("insert user: %w", err)
	}
	return u, nil
}

// GetByLogin ищет пользователя по логину. Если не найден — domain.ErrNotFound.
func (r *UserRepository) GetByLogin(ctx context.Context, login string) (domain.User, error) {
	var u domain.User
	err := r.pool.QueryRow(ctx,
		`SELECT id, login, password_hash, created_at FROM users WHERE login = $1`, login,
	).Scan(&u.ID, &u.Login, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user: %w", err)
	}
	return u, nil
}
