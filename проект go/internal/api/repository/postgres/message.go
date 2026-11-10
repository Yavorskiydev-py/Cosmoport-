package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmoport/internal/api/domain"
)

// MessageRepository работает с таблицей test_messages (ЛР2).
type MessageRepository struct {
	pool *pgxpool.Pool
}

// NewMessageRepository создаёт репозиторий тестовых сообщений.
func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

// Create записывает строку в БД.
func (r *MessageRepository) Create(ctx context.Context, text string) (domain.Message, error) {
	var m domain.Message
	err := r.pool.QueryRow(ctx,
		`INSERT INTO test_messages (text) VALUES ($1) RETURNING id, text, created_at`, text,
	).Scan(&m.ID, &m.Text, &m.CreatedAt)
	if err != nil {
		return domain.Message{}, fmt.Errorf("insert message: %w", err)
	}
	return m, nil
}

// List читает последние limit записей.
func (r *MessageRepository) List(ctx context.Context, limit int) ([]domain.Message, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, text, created_at FROM test_messages ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("select messages: %w", err)
	}
	defer rows.Close()

	messages := make([]domain.Message, 0, limit)
	for rows.Next() {
		var m domain.Message
		if err := rows.Scan(&m.ID, &m.Text, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
