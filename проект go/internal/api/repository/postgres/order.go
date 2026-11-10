package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmoport/internal/api/domain"
)

// OrderRepository работает с таблицей orders (ЛР4).
type OrderRepository struct {
	pool *pgxpool.Pool
}

// NewOrderRepository создаёт репозиторий заявок.
func NewOrderRepository(pool *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{pool: pool}
}

const orderColumns = `id, user_id, destination, passengers, price, status, status_reason, created_at, updated_at`

func scanOrder(row pgx.Row) (domain.Order, error) {
	var o domain.Order
	var status string
	err := row.Scan(&o.ID, &o.UserID, &o.Destination, &o.Passengers, &o.Price,
		&status, &o.StatusReason, &o.CreatedAt, &o.UpdatedAt)
	o.Status = domain.OrderStatus(status)
	return o, err
}

// Create сохраняет новую заявку.
func (r *OrderRepository) Create(ctx context.Context, o domain.Order) (domain.Order, error) {
	created, err := scanOrder(r.pool.QueryRow(ctx,
		`INSERT INTO orders (user_id, destination, passengers, price, status)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+orderColumns,
		o.UserID, o.Destination, o.Passengers, o.Price, string(o.Status)))
	if err != nil {
		return domain.Order{}, fmt.Errorf("insert order: %w", err)
	}
	return created, nil
}

// ListByUser возвращает заявки конкретного пользователя (новые сверху).
func (r *OrderRepository) ListByUser(ctx context.Context, userID int64, limit, offset int) ([]domain.Order, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE user_id = $1 ORDER BY id DESC LIMIT $2 OFFSET $3`,
		userID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("select orders: %w", err)
	}
	defer rows.Close()

	orders := make([]domain.Order, 0)
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan order: %w", err)
		}
		orders = append(orders, o)
	}
	return orders, rows.Err()
}

// GetByID возвращает заявку, только если она принадлежит пользователю userID.
func (r *OrderRepository) GetByID(ctx context.Context, id, userID int64) (domain.Order, error) {
	o, err := scanOrder(r.pool.QueryRow(ctx,
		`SELECT `+orderColumns+` FROM orders WHERE id = $1 AND user_id = $2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("select order: %w", err)
	}
	return o, nil
}

// statusRank — порядок статусов в жизненном цикле заявки.
const statusRank = `CASE %s WHEN 'created' THEN 0 WHEN 'processing' THEN 1 WHEN 'approved' THEN 2
	WHEN 'launched' THEN 3 ELSE 4 END`

// UpdateStatus меняет статус заявки. Статус может только двигаться вперёд по жизненному циклу:
// заявки в конечном статусе (completed/rejected) не меняются, а повторно доставленное
// старое событие (например, processing после approved) игнорируется.
// Возвращает domain.ErrNotFound, если заявки нет или обновление не применимо.
func (r *OrderRepository) UpdateStatus(ctx context.Context, id int64, status domain.OrderStatus, reason string) error {
	tag, err := r.pool.Exec(ctx,
		`UPDATE orders SET status = $2, status_reason = $3, updated_at = now()
		 WHERE id = $1 AND status NOT IN ('completed', 'rejected')
		   AND `+fmt.Sprintf(statusRank, "status")+` <= `+fmt.Sprintf(statusRank, "$2::text"),
		id, string(status), reason)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CountByStatus считает заявки по статусам (используется для метрик Prometheus).
func (r *OrderRepository) CountByStatus(ctx context.Context) (map[string]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT status, count(*) FROM orders GROUP BY status`)
	if err != nil {
		return nil, fmt.Errorf("count orders: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int64)
	for rows.Next() {
		var status string
		var n int64
		if err := rows.Scan(&status, &n); err != nil {
			return nil, err
		}
		counts[status] = n
	}
	return counts, rows.Err()
}
