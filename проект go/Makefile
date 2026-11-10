.PHONY: up down clean load logs ps test vet build demo

# Поднять всю систему (3 Go-сервиса + PostgreSQL + RabbitMQ + Prometheus + Grafana + Loki)
up:
	docker compose up -d --build

# Остановить систему (данные сохраняются)
down:
	docker compose --profile load down

# Остановить и удалить все данные (БД, очереди, метрики)
clean:
	docker compose --profile load down -v

# Запустить нагрузочный тест: make load USERS=60 DURATION=3m
USERS ?= 40
DURATION ?= 5m
load:
	LOAD_USERS=$(USERS) LOAD_DURATION=$(DURATION) docker compose --profile load up -d --build loadgen

logs:
	docker compose logs -f api mission-control loadgen

ps:
	docker compose ps

test:
	go test -race -count=1 ./...

vet:
	go vet ./...

build:
	go build -o bin/ ./cmd/...

# Быстрая ручная проверка всех эндпоинтов
demo:
	./scripts/demo.sh
