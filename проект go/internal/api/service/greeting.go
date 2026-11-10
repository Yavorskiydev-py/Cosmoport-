package service

import "context"

// GreetingService — бизнес-логика эндпоинта /test (ЛР1).
type GreetingService struct {
	repo GreetingRepository
}

// NewGreetingService создаёт сервис приветствия.
func NewGreetingService(repo GreetingRepository) *GreetingService {
	return &GreetingService{repo: repo}
}

// Hello возвращает приветствие, полученное из слоя repository.
func (s *GreetingService) Hello(ctx context.Context) (string, error) {
	return s.repo.Greeting(ctx)
}
