package service

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"cosmoport/internal/api/domain"
)

const maxMessageLength = 1000

// MessageService — бизнес-логика эндпоинта /dbtest (ЛР2).
type MessageService struct {
	repo MessageRepository
}

// NewMessageService создаёт сервис тестовых сообщений.
func NewMessageService(repo MessageRepository) *MessageService {
	return &MessageService{repo: repo}
}

// Save проверяет строку и записывает её в БД.
func (s *MessageService) Save(ctx context.Context, text string) (domain.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Message{}, fmt.Errorf("%w: request body must not be empty", domain.ErrValidation)
	}
	if utf8.RuneCountInString(text) > maxMessageLength {
		return domain.Message{}, fmt.Errorf("%w: text is longer than %d characters", domain.ErrValidation, maxMessageLength)
	}
	return s.repo.Create(ctx, text)
}

// Latest возвращает последние сохранённые строки.
func (s *MessageService) Latest(ctx context.Context) ([]domain.Message, error) {
	return s.repo.List(ctx, 20)
}
