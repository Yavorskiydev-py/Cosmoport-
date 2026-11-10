// Package memory — реализации репозиториев, хранящих данные в памяти.
package memory

import "context"

// GreetingRepository отдаёт приветствие для эндпоинта /test (ЛР1).
// Это самый нижний (третий) слой цепочки handler -> service -> repository.
type GreetingRepository struct {
	greeting string
}

// NewGreetingRepository создаёт репозиторий с приветствием "Hello!".
func NewGreetingRepository() *GreetingRepository {
	return &GreetingRepository{greeting: "Hello!"}
}

// Greeting возвращает строку приветствия.
func (r *GreetingRepository) Greeting(_ context.Context) (string, error) {
	return r.greeting, nil
}
