# 🚀 Cosmoport — сервис бронирования космических полётов на Go

Учебный проект по курсу Go. Пользователь регистрируется, получает JWT-токен и оформляет
заявку на полёт: на МКС, Луну, Марс, Европу или Титан. Отдельный сервис **«Центр управления
полётами» (mission-control)** получает заявку через **RabbitMQ**, имитирует подготовку
и полёт и сообщает об изменении статуса обратно. Вся система разворачивается одной
командой через **Docker Compose**, мониторится через **Prometheus + Grafana (+ Loki для логов)**
и проверяется собственным **нагрузочным сервисом**.

> Проект сделан по образцу `2248_FoodDeliveryService`. Вместо «доставки еды» у нас
> «доставка космонавтов». Заказ — это заявка на полёт, а сервис обработки — ЦУП.

---

## Содержание

1. [Соответствие критериям оценки](#1-соответствие-критериям-оценки)
2. [Быстрый старт](#2-быстрый-старт)
   * [2.1. Веб-интерфейс](#21-веб-интерфейс)
3. [Архитектура](#3-архитектура)
4. [Лабораторные работы 1–5: что и где сделано](#4-лабораторные-работы-15-что-и-где-сделано)
   * [4.1. Пошагово руками: от регистрации до полёта](#41-пошагово-руками-от-регистрации-до-полёта)
5. [Сервис обработки и RabbitMQ (оценка 6–7)](#5-сервис-обработки-и-rabbitmq-оценка-67)
6. [Docker Compose, Prometheus, Grafana, логи (оценка 8)](#6-docker-compose-prometheus-grafana-логи-оценка-8)
7. [Нагрузочный сервис и дашборды (оценка 9–10)](#7-нагрузочный-сервис-и-дашборды-оценка-910)
8. [Справочник API](#8-справочник-api)
9. [Структура проекта](#9-структура-проекта)
10. [Тесты](#10-тесты)
11. [Сценарий защиты (что показывать преподавателю)](#11-сценарий-защиты-что-показывать-преподавателю)
12. [Вопросы, которые могут задать на защите](#12-вопросы-которые-могут-задать-на-защите)
13. [Решение проблем](#13-решение-проблем)

---

## 1. Соответствие критериям оценки

| Оценка | Требование | Где реализовано |
|---|---|---|
| ЛР1 | HTTP-сервер, `GET /test` → `Hello!`, 3 слоя через интерфейсы, graceful shutdown | `internal/api/handler/basic.go`, `service/greeting.go`, `repository/memory/greeting.go`, `internal/api/app/app.go` |
| ЛР2 | PostgreSQL, инициализация таблиц, `POST /dbtest` пишет тело запроса в БД, работа с БД в слое repository | `repository/postgres/*`, `repository/postgres/migrations/001_init.sql` |
| ЛР3 | Регистрация (пользователь в БД), авторизация с выдачей JWT | `handler/auth.go`, `service/auth.go`, `internal/api/auth/jwt.go` |
| ЛР4 | Создание заявки, просмотр статусов всех заявок пользователя | `handler/order.go`, `service/order.go`, `repository/postgres/order.go` |
| ЛР5 | Middleware: принимает JWT, передаёт ID пользователя в хэндлеры, у каждого пользователя свои заявки | `internal/api/middleware/auth.go` |
| **4–5** | Основная часть сервера (ЛР1–ЛР5) | `cmd/api` + `internal/api` |
| **6–7** | Отдельный сервис обработки, который меняет статусы. Связь через RabbitMQ в обе стороны | `cmd/mission-control`, `internal/mission`, `internal/api/messaging`, `internal/broker` |
| **8** | Prometheus + Grafana, деплой через Docker Compose | `docker-compose.yml`, `Dockerfile`, `deploy/prometheus`, `deploy/grafana`, `deploy/promtail` |
| **9–10** | Нагрузочный сервис, демонстрация работоспособности на дашбордах Grafana | `cmd/loadgen`, `internal/loadgen`, `deploy/grafana/dashboards/cosmoport.json` |

Дополнительно (сверх требований): юнит-тесты, сбор логов через Loki, экспортер метрик
PostgreSQL, метрики RabbitMQ, подтверждения публикации (publisher confirms), защита
от «отката» статусов, возврат незавершённых миссий в очередь при остановке, request-id,
перехват паник, пагинация.

---

## 2. Быстрый старт

### Что нужно установить

* **Docker Desktop**: для запуска всей системы. Больше ничего не требуется.
* **Go 1.26+**: только если хочешь запускать тесты или сервисы без Docker.

### Запуск

```bash
cd "проект go"
docker compose up -d --build        # или: make up
```

Первая сборка занимает 2–5 минут, потому что скачиваются образы. Проверка состояния:

```bash
docker compose ps
```

Все сервисы должны быть в статусе `Up`, а `api`, `postgres` и `rabbitmq` ещё и `(healthy)`.

### Что где открыть

| Что | Адрес | Логин / пароль |
|---|---|---|
| **🖥️ Веб-интерфейс (начни отсюда)** | **http://localhost:28080** | регистрируешься прямо в нём |
| API «Космопорта» | http://localhost:28080/test | — |
| **Grafana (дашборд)** | http://localhost:23000 | `admin` / `admin` (можно и без входа) |
| Prometheus | http://localhost:29090/targets | — |
| RabbitMQ (веб-интерфейс) | http://localhost:27673 | `cosmoport` / `cosmoport` |
| Метрики api | http://localhost:28080/metrics | — |
| Метрики mission-control | http://localhost:28081/metrics | — |
| PostgreSQL | `localhost:25432`, БД `cosmoport` | `cosmoport` / `cosmoport` |

> **Порты.** Все внешние порты заданы в файле `.env`. Специально выбран диапазон **2xxxx**
> (API 28080, Grafana 23000 и т.д.), а не стандартные 3000/8080/5432/9090: они часто заняты
> сайтами и другими проектами. Если какой-то порт всё же занят,
> поменяй его в `.env` и выполни `docker compose up -d`. Внутри Docker-сети сервисы общаются
> по своим внутренним портам, поэтому больше ничего менять не нужно.

| Переменная в `.env` | Порт | Что |
|---|---|---|
| `API_PORT` | 28080 | основной сервис api |
| `MISSION_CONTROL_PORT` | 28081 | метрики ЦУП |
| `GRAFANA_PORT` | 23000 | Grafana |
| `PROMETHEUS_PORT` | 29090 | Prometheus |
| `LOKI_PORT` | 23100 | Loki |
| `RABBITMQ_PORT` / `RABBITMQ_UI_PORT` | 27672 / 27673 | RabbitMQ (AMQP / веб-интерфейс) |
| `POSTGRES_PORT` | 25432 | PostgreSQL |
| `LOADGEN_PORT` | 29100 | метрики loadgen |

### 2.1. Веб-интерфейс

Открой **http://localhost:28080**. Всё, что в README делается через `curl`, здесь можно
сделать кнопками и увидеть своими глазами. Интерфейс работает через тот же REST API,
никаких «секретных» эндпоинтов у него нет. Его отдаёт сам сервис `api`: файлы HTML, CSS
и JS встроены в бинарник через `go:embed` (`internal/api/web`).

**Вкладка «Полёты»** (ЛР3–ЛР5 и связь через RabbitMQ):

1. Слева «Вход в космопорт»: переключись на «Регистрация», введи логин и пароль и нажми
   кнопку. Интерфейс зарегистрирует пользователя и сразу выполнит вход. Под «Показать
   JWT-токен» виден сам токен и его содержимое (`sub` = ID пользователя, `exp` = срок).
2. «Куда летим?»: кликни по планете, укажи число пассажиров и нажми **«🚀 Забронировать полёт»**.
3. «Мои заявки»: у заявки шкала полёта, и статус двигается сам каждые 1–3 секунды
   (`Принята → Подготовка → Одобрено → В полёте → Прибыли` или `Отклонено`).
   Справа всплывают уведомления.
4. «Как это работает»: схема подсвечивает путь сообщения. При создании заявки сигнал идёт
   браузер → api → PostgreSQL и RabbitMQ → ЦУП, при смене статуса обратно ЦУП →
   RabbitMQ → api → PostgreSQL.

![Вкладка «Полёты»](docs/ui-flights.png)

**Вкладка «Проверка ЛР»**: кнопки для ЛР1 (`GET /test`, подсвечиваются 3 слоя), ЛР2
(запись строки в PostgreSQL и список последних записей) и ЛР5 (запрос без токена,
с поддельным и с настоящим токеном: видно `401` и `200`), а также команды для
демонстрации graceful shutdown.

![Вкладка «Проверка ЛР»](docs/ui-labs.png)

**Вкладка «Мониторинг»**: главные графики Grafana встроены прямо в страницу, рядом кнопки
перехода в Grafana, Prometheus и RabbitMQ. Чтобы графики ожили, запусти нагрузку
(`docker compose --profile load up -d loadgen`).

![Вкладка «Мониторинг»](docs/ui-monitoring.png)

**Консоль запросов** внизу экрана показывает каждый запрос интерфейса к API: метод, путь,
код ответа и время. Клик по строке раскрывает заголовки (виден `Authorization: Bearer …`),
тело запроса и ответ сервера. Голубые строки — события: «заявка ушла в очередь
orders.created», «ЦУП прислал статус». Автообновление списка заявок скрыто, чтобы не
засорять консоль, но его можно включить галочкой.

Индикатор **«api в строю»** в правом верхнем углу проверяет `/health` каждые 3 секунды.
Если выполнить `docker compose stop api`, он станет красным, а после
`docker compose start api` снова зелёным.

### Проверить все лабораторные одной командой

```bash
./scripts/demo.sh        # или: make demo
```

### Запустить нагрузочный тест

```bash
docker compose --profile load up -d loadgen          # 40 пользователей, 5 минут
# или с параметрами:
LOAD_USERS=80 LOAD_DURATION=3m docker compose --profile load up -d loadgen
# или: make load USERS=80 DURATION=3m
```

Затем открой Grafana: дашборд **«🚀 Cosmoport — мониторинг космопорта»** открывается
как домашняя страница.

### Остановка

```bash
docker compose --profile load down       # остановить (данные сохранятся)
docker compose --profile load down -v    # остановить и удалить все данные
```

---

## 3. Архитектура

### Общая схема системы

```mermaid
flowchart LR
    U[Пользователь / curl] -->|HTTP + JWT| API
    LG[loadgen<br/>нагрузочный сервис] -->|HTTP + JWT| API

    subgraph API[api — основной сервис :8080]
        H[handler] --> S[service] --> R[repository]
    end

    R -->|SQL| PG[(PostgreSQL)]
    S -->|orders.created| MQ{{RabbitMQ}}
    MQ -->|orders.created| MC[mission-control<br/>ЦУП :8081]
    MC -->|orders.status| MQ
    MQ -->|orders.status| API

    PROM[Prometheus] -.->|/metrics| API
    PROM -.-> MC
    PROM -.-> LG
    PROM -.-> MQ
    PROM -.-> PGE[postgres-exporter] -.-> PG
    PT[Promtail] -.->|логи контейнеров| LOKI[Loki]
    GR[Grafana] --> PROM
    GR --> LOKI
```

### Чистая архитектура основного сервиса (3 слоя)

```
 HTTP-запрос
     │
     ▼
┌────────────────────┐   разбирает запрос, проверяет JSON, превращает ошибки в HTTP-коды
│  handler (слой 1)  │   internal/api/handler
└─────────┬──────────┘
          │ интерфейсы GreetingService, AuthService, OrderService ...
          ▼
┌────────────────────┐   бизнес-логика: валидация, хэширование паролей, расчёт цены,
│  service (слой 2)  │   отправка событий в брокер. Ничего не знает про HTTP и SQL
└─────────┬──────────┘   internal/api/service
          │ интерфейсы UserRepository, OrderRepository, EventPublisher ...
          ▼
┌────────────────────┐   работа с хранилищем: SQL-запросы к PostgreSQL
│ repository (слой 3)│   internal/api/repository/postgres (+ memory для /test)
└────────────────────┘
```

**Изоляция через интерфейсы.** Каждый слой зависит не от конкретной реализации
нижнего слоя, а от интерфейса. Интерфейсы объявлены в том пакете, который их
*использует*: интерфейсы сервисов лежат в `handler/handler.go`, интерфейсы репозиториев
в `service/interfaces.go`. Конкретные объекты создаются и соединяются в одном месте,
в `internal/api/app/app.go` (dependency injection). Поэтому в тестах сервисы
проверяются с «фейковыми» репозиториями, без настоящей БД.

Общие сущности (`User`, `Order`, статусы, ошибки) лежат в `internal/api/domain`.
Этот пакет не зависит ни от одного слоя.

### Жизненный цикл заявки

```mermaid
stateDiagram-v2
    [*] --> created: POST /orders
    created --> processing: ЦУП принял заявку
    processing --> approved: экипаж прошёл проверку (~90%)
    processing --> rejected: солнечная вспышка, медкомиссия... (~10%)
    approved --> launched: старт ракеты
    launched --> completed: прибытие
    rejected --> [*]
    completed --> [*]
```

### Последовательность обработки заявки

```mermaid
sequenceDiagram
    participant C as Клиент
    participant A as api
    participant DB as PostgreSQL
    participant Q as RabbitMQ
    participant M as mission-control

    C->>A: POST /orders (Bearer JWT)
    A->>A: middleware достаёт user_id из токена
    A->>DB: INSERT orders (status=created)
    A->>Q: publish orders.created
    A-->>C: 201 {id, status: created}
    Q->>M: orders.created
    loop каждый этап (1–3 сек)
        M->>Q: publish orders.status (processing / approved / launched / completed)
        Q->>A: orders.status
        A->>DB: UPDATE orders SET status=...
    end
    C->>A: GET /orders
    A-->>C: список заявок с актуальными статусами
```

---

## 4. Лабораторные работы 1–5: что и где сделано

### ЛР1: пустой HTTP-сервер, `/test`, graceful shutdown

* `GET /test` возвращает `Hello!`. Запрос проходит **все три слоя**:
  `handler.test` → `GreetingService.Hello` → `GreetingRepository.Greeting`.
  Репозиторий здесь хранит строку в памяти (`repository/memory`), потому что в ЛР1 базы ещё нет.
* **Graceful shutdown** (`cmd/api/main.go` + `internal/api/app/app.go`):
  1. `signal.NotifyContext` ловит `SIGINT` (Ctrl+C) и `SIGTERM` (`docker stop`).
  2. `http.Server.Shutdown` перестаёт принимать новые соединения и **дожидается окончания
     текущих запросов** (таймаут `SHUTDOWN_TIMEOUT`, по умолчанию 15 с).
  3. Останавливается потребитель RabbitMQ. Сообщение, которое он обрабатывает, доводится до конца.
  4. Закрываются соединение с RabbitMQ и пул соединений с PostgreSQL.
  5. В логе появляется `service stopped gracefully`.
* У сервера заданы таймауты (`ReadHeaderTimeout`, `ReadTimeout`, `WriteTimeout`,
  `IdleTimeout`). Без них медленный клиент может держать соединение бесконечно.

Как проверить:

```bash
curl http://localhost:28080/test          # Hello!
docker compose stop api                  # graceful shutdown
docker compose logs api | tail           # shutdown signal received ... service stopped gracefully
docker compose start api
```

### ЛР2: PostgreSQL и `/dbtest`

* Подключение: `repository/postgres/postgres.go`. Используется пул `pgxpool`, при старте
  выполняются повторные попытки, пока БД не станет доступна.
* **Инициализация таблиц:** `repository/postgres/migrate.go` применяет SQL-файлы из
  `migrations/` (они встроены в бинарник через `//go:embed`). Применённые версии
  записываются в таблицу `schema_migrations`, поэтому повторный запуск ничего не ломает.
  `pg_advisory_lock` не даёт двум копиям сервиса накатывать миграции одновременно.
* `POST /dbtest` записывает **строку из тела запроса** в таблицу `test_messages`.
  `GET /dbtest` показывает последние записи, то есть проверяет и чтение.
* Весь SQL лежит **только** в слое repository.

```bash
curl -X POST http://localhost:28080/dbtest -d 'Поехали!'
# {"id":1,"text":"Поехали!","created_at":"..."}
curl http://localhost:28080/dbtest
```

### ЛР3: регистрация и авторизация (JWT)

* `POST /auth/register`: создаёт пользователя в таблице `users`. Пароль хранится
  **только как bcrypt-хэш**. Логин уникален: повторная регистрация вернёт `409 Conflict`.
* `POST /auth/login`: проверяет пароль и выдаёт **сессионный JWT-токен** (HS256,
  срок жизни `JWT_TTL` = 24 ч). В поле `sub` токена записан ID пользователя.
* Валидация: логин 3–32 символа (`a-z A-Z 0-9 _ . -`), пароль 6–72 символа.
* Если логина нет, сервер всё равно выполняет сравнение bcrypt с хэшем-заглушкой.
  Поэтому по времени ответа нельзя узнать, существует ли такой пользователь.

```bash
curl -X POST http://localhost:28080/auth/register -d '{"login":"gagarin","password":"vostok-1"}'
curl -X POST http://localhost:28080/auth/login    -d '{"login":"gagarin","password":"vostok-1"}'
# {"token":"eyJhbGciOi...","token_type":"Bearer","expires_at":"..."}
```

### ЛР4: заявки (заказы)

* `POST /orders`: создать заявку на полёт `{"destination":"mars","passengers":2}`.
  Цена считается автоматически: цена места × число пассажиров.
* `GET /orders`: **все заявки текущего пользователя и их статусы** (новые сверху,
  пагинация `?limit=&offset=`).
* `GET /orders/{id}`: одна заявка. Чужую заявку получить нельзя, сервер ответит `404`.
* `GET /destinations`: каталог направлений.

### ЛР5: middleware авторизации

`internal/api/middleware/auth.go`:

1. Берёт заголовок `Authorization: Bearer <token>`.
2. Проверяет подпись, алгоритм, издателя и срок действия токена (`auth.JWTManager.Parse`).
3. Кладёт ID пользователя в `context` запроса.
4. Хэндлеры достают его через `middleware.UserIDFromContext(r.Context())`.

Без токена или с неверным токеном сервер отвечает `401`. В SQL-запросах всегда есть
условие `WHERE user_id = $1`, поэтому **у каждого пользователя свой список заявок**.

Пошаговая инструкция с готовыми командами: [раздел 4.1](#41-пошагово-руками-от-регистрации-до-полёта).

### 4.1. Пошагово руками: от регистрации до полёта

Команды выполняются в терминале **по очереди, в одном и том же окне**. Переменная
`TOKEN` живёт только в том окне, где её задали. Если открыть новое окно, шаг 3 нужно
повторить.

**Шаг 1. Регистрация.** Придумай логин и пароль (пароль не короче 6 символов):

```bash
curl -X POST http://localhost:28080/auth/register -d '{"login":"korolev","password":"vostok-61"}'
```

Ответ `{"id":146,"login":"korolev",...}` значит, что пользователь создан в БД.
Ответ `{"error":"already exists"}` значит, что такой логин уже есть: возьми другой
или переходи к шагу 2 с этим логином.

**Шаг 2. Вход: посмотреть на токен.**

```bash
curl -X POST http://localhost:28080/auth/login -d '{"login":"korolev","password":"vostok-61"}'
```

Ответ: `{"token":"eyJhbGciOiJIUzI1NiIs...длинная строка...","token_type":"Bearer",...}`.
Длинная строка в `token` и есть **JWT-токен**, «пропуск» пользователя. Он действует 24 часа.

**Шаг 3. Сохранить токен в переменную**, чтобы не копировать его руками:

```bash
TOKEN=$(curl -s -X POST http://localhost:28080/auth/login \
  -d '{"login":"korolev","password":"vostok-61"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

echo $TOKEN      # проверка: должна напечататься длинная строка eyJ...
```

> Можно и вручную: скопировать токен из ответа шага 2 (без кавычек) и выполнить
> `TOKEN=eyJhbGciOi...вставь_сюда...`

**Шаг 4. Посмотреть, куда можно полететь** (токен не нужен):

```bash
curl http://localhost:28080/destinations
```

Коды направлений: `iss`, `moon`, `mars`, `europa`, `titan`.

**Шаг 5. Создать заявку на полёт.** Токен передаётся в заголовке `Authorization`:

```bash
curl -X POST http://localhost:28080/orders \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"destination":"moon","passengers":2}'
```

Ответ: `{"id":39888,"user_id":146,"destination":"moon","passengers":2,"price":300000,"status":"created",...}`.
Заявка сохранена в БД со статусом `created`, а `api` отправил о ней сообщение в
RabbitMQ для ЦУП.

**Шаг 6. Посмотреть свои заявки и их статусы.** Повтори команду несколько раз
с интервалом в пару секунд:

```bash
curl http://localhost:28080/orders -H "Authorization: Bearer $TOKEN"
```

Статус будет меняться у тебя на глазах: `created` → `processing` → `approved` →
`launched` → `completed`. Примерно в 10% случаев вместо этого будет `rejected` с причиной
в `status_reason`, например «Солнечная вспышка». Эти статусы меняет сервис
`mission-control` и присылает их через RabbitMQ. Весь полёт занимает около 3–9 секунд.

Красивый вывод (каждое поле на своей строке):

```bash
curl -s http://localhost:28080/orders -H "Authorization: Bearer $TOKEN" | python3 -m json.tool --no-ensure-ascii
```

Одна заявка по её `id` (подставь свой номер из шага 5):

```bash
curl http://localhost:28080/orders/39888 -H "Authorization: Bearer $TOKEN"
```

**Шаг 7. Проверить защиту (ЛР5).**

```bash
curl http://localhost:28080/orders                                     # без токена → 401
curl http://localhost:28080/orders -H "Authorization: Bearer abc"      # поддельный токен → 401
```

**Шаг 8. Проверить, что у каждого пользователя свои заявки.** Зарегистрируй второго
пользователя и получи его токен в переменную `TOKEN2`:

```bash
curl -X POST http://localhost:28080/auth/register -d '{"login":"tereshkova","password":"chaika-63"}'
TOKEN2=$(curl -s -X POST http://localhost:28080/auth/login \
  -d '{"login":"tereshkova","password":"chaika-63"}' \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["token"])')

curl http://localhost:28080/orders -H "Authorization: Bearer $TOKEN2"          # [] — у неё заявок нет
curl http://localhost:28080/orders/39888 -H "Authorization: Bearer $TOKEN2"    # чужая заявка → 404
```

**Частые ошибки:**

| Что видишь | Почему | Что делать |
|---|---|---|
| `zsh: command not found: url` | потерялась буква `c` в `curl` | проверь команду |
| `401` и `missing or malformed Authorization header` на шаге 5 | переменная `TOKEN` пустая (новое окно терминала или шаг 3 не выполнен) | `echo $TOKEN`, при пустом выводе повтори шаг 3 |
| `401 invalid or expired token` | токен скопирован не целиком или прошло 24 часа | повтори шаг 3 |
| `400 unknown destination` | опечатка в направлении | используй `iss`, `moon`, `mars`, `europa`, `titan` |
| `400 passengers must be between 1 and 10` | неверное число пассажиров | от 1 до 10 |
| статус долго остаётся `created` | не запущен `mission-control` | `docker compose ps`, затем `docker compose up -d` |

Все эти шаги выполняются и автоматически, одной командой: `./scripts/demo.sh`.

Остальные middleware (`middleware/observability.go`): `Recover` перехватывает панику
и отвечает 500 вместо падения сервера, `RequestID` добавляет запросу заголовок
`X-Request-ID`, `Observe` пишет лог по каждому запросу и метрики Prometheus.

---

## 5. Сервис обработки и RabbitMQ (оценка 6–7)

**mission-control** — это отдельный Go-сервис (`cmd/mission-control`, отдельный контейнер).
Своей БД у него нет, он общается с основным сервисом **только через RabbitMQ**.

| Очередь | Кто пишет | Кто читает | Сообщение |
|---|---|---|---|
| `orders.created` | api (после создания заявки) | mission-control | `{"order_id", "user_id", "destination", "passengers", "created_at"}` |
| `orders.status` | mission-control (на каждом этапе) | api (обновляет БД) | `{"order_id", "status", "reason", "changed_at"}` |

Форматы сообщений описаны в одном месте, в `internal/contracts/events.go`.

**Как работает ЦУП** (`internal/mission/processor.go`):

* Пул воркеров (`WORKERS`, по умолчанию 1000 горутин). Каждый воркер берёт заявку из очереди.
* Имитирует этапы `processing → approved → launched → completed`. Каждый этап длится
  случайное время от `MIN_STAGE_DELAY` до `MAX_STAGE_DELAY` (1–3 с).
  С вероятностью `REJECT_RATE` (10%) полёт отклоняется со случайной причиной,
  например «Солнечная вспышка».
* Каждое изменение статуса публикуется в `orders.status`.
* Сообщение подтверждается (`Ack`) **только после завершения всей миссии**.
  Если сервис остановить посреди миссии, она возвращается в очередь (`Nack` + requeue)
  и после перезапуска обрабатывается заново. **Сообщения не теряются.**

**Надёжность:**

* Очереди `durable`, сообщения `persistent`: они переживают перезапуск RabbitMQ.
* **Publisher confirms**: публикация считается успешной, только когда брокер её подтвердил.
* Ручные подтверждения (`autoAck=false`) и `prefetch` (QoS) у потребителей.
* Статус может двигаться **только вперёд**. Повторно доставленное старое событие,
  например `processing` после `approved`, игнорируется SQL-условием в `UpdateStatus`.
* Если брокер недоступен в момент создания заявки, она всё равно сохраняется и
  возвращается клиенту. Ошибка попадает в логи и в метрику
  `cosmoport_events_published_total{result="failure"}`.

Посмотреть очереди можно в веб-интерфейсе RabbitMQ: http://localhost:27673 → вкладка *Queues*.

---

## 6. Docker Compose, Prometheus, Grafana, логи (оценка 8)

### Деплой

`docker compose up -d --build` поднимает **9 контейнеров**. Десятый, `loadgen`, запускается
отдельной командой, когда нужна нагрузка:

| Сервис | Образ | Назначение |
|---|---|---|
| `api` | собирается из `Dockerfile` | основной сервис |
| `mission-control` | собирается из `Dockerfile` | ЦУП (обработка заявок) |
| `loadgen` | собирается из `Dockerfile` | нагрузка (профиль `load`, запускается отдельно) |
| `postgres` | `postgres:16-alpine` | база данных |
| `rabbitmq` | `rabbitmq:3.13-management-alpine` | брокер сообщений, включён плагин `rabbitmq_prometheus` |
| `prometheus` | `prom/prometheus` | сбор метрик каждые 5 с |
| `postgres-exporter` | `prometheuscommunity/postgres-exporter` | метрики PostgreSQL |
| `loki` | `grafana/loki` | хранилище логов |
| `promtail` | `grafana/promtail` | собирает логи контейнеров и отправляет в Loki |
| `grafana` | `grafana/grafana` | дашборды, источники данных и дашборд подключаются автоматически |

Особенности:

* **Один Dockerfile на три сервиса**: нужный выбирается аргументом `SERVICE`.
  Сборка многоэтапная: сначала `golang:1.26-alpine`, финальный образ `alpine` размером около 20 МБ,
  процесс запускается не от root.
* `depends_on` с `condition: service_healthy`: api стартует только после того,
  как PostgreSQL и RabbitMQ готовы. Есть `healthcheck` на каждый важный сервис.
* `stop_grace_period`: Docker даёт сервисам время на graceful shutdown.

### Метрики (Prometheus)

Каждый Go-сервис отдаёт метрики на `GET /metrics`. Prometheus опрашивает их по
конфигу `deploy/prometheus/prometheus.yml`. Цели можно увидеть на
http://localhost:29090/targets, все должны быть в состоянии **UP**.

| Метрика | Что показывает |
|---|---|
| `cosmoport_http_requests_total{route,method,code}` | количество запросов |
| `cosmoport_http_request_duration_seconds` | гистограмма времени ответа (p50/p95/p99) |
| `cosmoport_http_requests_in_flight` | запросов в обработке прямо сейчас |
| `cosmoport_users_registered_total`, `cosmoport_logins_total{result}` | регистрации и входы |
| `cosmoport_orders_created_total{destination}` | созданные заявки по направлениям |
| `cosmoport_orders_by_status{status}` | сколько заявок в каждом статусе (считается из БД) |
| `cosmoport_order_status_updates_total{status}` | применённые изменения статусов |
| `cosmoport_events_published_total{queue,result}` | публикации в RabbitMQ |
| `cosmoport_db_pool_*` | пул соединений с БД |
| `mission_control_missions_in_progress` | миссий в работе |
| `mission_control_status_transitions_total{status}` | изменения статусов, отправленные ЦУП |
| `mission_control_mission_duration_seconds` | длительность миссии |
| `loadgen_requests_total{op,code}`, `loadgen_request_duration_seconds` | метрики со стороны нагрузки |
| `rabbitmq_queue_messages_ready{queue}` | глубина очередей |
| `pg_stat_database_*` | транзакции и операции PostgreSQL |
| `go_goroutines`, `go_memstats_*`, `process_cpu_seconds_total` | ресурсы Go-сервисов |

В метке `route` хранится шаблон маршрута (`GET /orders/{id}`), а не реальный путь.
Иначе каждый ID заявки создавал бы новый временной ряд в Prometheus.

### Логирование

* Все сервисы пишут **структурированные JSON-логи** (`log/slog`) в stdout. В каждой
  записи есть поле `service`, а в логах HTTP-запросов ещё `request_id`, `status`
  и `duration_ms`.
* **Promtail** находит контейнеры с меткой `logging=promtail`, читает их логи и
  отправляет в **Loki**.
* В **Grafana** логи видны внизу дашборда. Можно фильтровать по сервису и искать по
  тексту, например `order_id":42` или `ERROR`.

---

## 7. Нагрузочный сервис и дашборды (оценка 9–10)

### loadgen

`cmd/loadgen` запускает N **виртуальных космонавтов**. Они стартуют постепенно,
в течение `RAMP_UP`. Каждый космонавт:

1. регистрируется (`cosmonaut_<run>_<n>`);
2. входит и получает JWT;
3. в цикле, с паузой 100–500 мс («время на подумать»), выполняет случайное действие:

| Действие | Доля | Ожидаемый ответ |
|---|---|---|
| создать заявку `POST /orders` | 30% | 201 |
| список заявок `GET /orders` | 35% | 200 |
| статус одной заявки `GET /orders/{id}` | 15% | 200 |
| каталог `GET /destinations` | 10% | 200 |
| `GET /test` | 5% | 200 |
| запрос с **неверным токеном** | 5% | 401 (проверка ЛР5 под нагрузкой) |

Если ответ не совпадает с ожидаемым, он считается в `loadgen_unexpected_responses_total`.
В конце теста в лог выводится итог: число запросов, ошибок, созданных заявок и средний RPS.

Параметры задаются переменными окружения или флагами:

```bash
LOAD_USERS=100 LOAD_DURATION=5m LOAD_RAMP_UP=30s docker compose --profile load up -d loadgen
# локально без Docker:
go run ./cmd/loadgen -target http://localhost:28080 -users 50 -duration 2m
docker compose logs loadgen | grep finished       # итог теста
```

### Дашборд Grafana

Файл `deploy/grafana/dashboards/cosmoport.json` подключается автоматически
(provisioning). Обновляется каждые 5 секунд. Разделы:

1. **🚀 Обзор системы**: RPS, задержка p95, процент ошибок 5xx, число пользователей и
   заявок, миссии в работе, статус UP/DOWN каждого компонента, число виртуальных пользователей.
2. **🌐 HTTP API**: RPS по маршрутам, задержки p50/p95/p99, коды ответов, p95 по маршрутам.
3. **🛰️ Заявки и ЦУП**: заявки по направлениям, заявки по статусам, сравнение «ЦУП
   отправил» и «api применил» (видно, что обе стороны RabbitMQ работают синхронно),
   длительность миссий, глубина очередей, публикации событий.
4. **🔥 Нагрузочный тест**: RPS по операциям, задержка глазами клиента, ошибки клиента.
5. **⚙️ Ресурсы**: горутины, память, CPU каждого сервиса, пул соединений с БД, транзакции PostgreSQL.
6. **📜 Логи (Loki)**: живые логи всех сервисов с фильтром и поиском, график логов по уровням.

![Дашборд Grafana под нагрузкой](docs/grafana-dashboard.png)

На скриншоте видно два прогона нагрузки. В первом у ЦУП было всего 50 воркеров:
заявки поступали быстрее, чем обрабатывались, и очередь `orders.created` выросла
до 13 тыс. сообщений (график «Очереди RabbitMQ»). После увеличения числа воркеров
во втором прогоне очередь держится около нуля. Именно для таких выводов и нужен мониторинг.

### Результаты проверки (40–60 виртуальных пользователей)

* 40 пользователей с паузами 100–500 мс дают около **130 RPS** и ~38 заявок/с;
  60 пользователей с паузами 50–300 мс дают около **335 RPS** и ~100 заявок/с;
* задержка **p95 ≈ 4 мс**, p99 < 10 мс;
* **0 ошибок 5xx**, все неожиданные ответы были только в моменты, когда api специально останавливали;
* очереди RabbitMQ не накапливаются: ЦУП успевает за потоком заявок;
* при `docker compose stop api` / `stop mission-control` под нагрузкой оба сервиса
  завершаются корректно, а незавершённые миссии возвращаются в очередь.

---

## 8. Справочник API

Все ответы в формате JSON (кроме `/test`). Ошибки возвращаются как `{"error": "текст"}`.

| Метод | Путь | Авторизация | Тело запроса | Ответ |
|---|---|---|---|---|
| GET | `/test` | — | — | `200 Hello!` |
| POST | `/dbtest` | — | любая строка | `201 {id, text, created_at}` |
| GET | `/dbtest` | — | — | `200 [последние 20 строк]` |
| POST | `/auth/register` | — | `{"login","password"}` | `201 {id, login, created_at}` / `400` / `409` |
| POST | `/auth/login` | — | `{"login","password"}` | `200 {token, token_type, expires_at}` / `401` |
| GET | `/destinations` | — | — | `200 [каталог направлений]` |
| POST | `/orders` | Bearer | `{"destination":"moon","passengers":1}` | `201 {заявка}` / `400` / `401` |
| GET | `/orders?limit=50&offset=0` | Bearer | — | `200 [заявки пользователя]` / `401` |
| GET | `/orders/{id}` | Bearer | — | `200 {заявка}` / `404` / `401` |
| GET | `/health` | — | — | `200 {"status":"ok"}` (проверяет БД) |
| GET | `/metrics` | — | — | метрики Prometheus |

Направления: `iss` (МКС), `moon` (Луна), `mars` (Марс), `europa` (Европа), `titan` (Титан).
Пассажиров: от 1 до 10.

Пример заявки:

```json
{
  "id": 1,
  "user_id": 1,
  "destination": "mars",
  "passengers": 2,
  "price": 2400000,
  "status": "launched",
  "status_reason": "Ключ на старт! Ракета покинула стартовый стол",
  "created_at": "2026-10-11T21:37:34.517149Z",
  "updated_at": "2026-10-11T21:37:39.104511Z"
}
```

### Переменные окружения

| Сервис | Переменная | По умолчанию | Назначение |
|---|---|---|---|
| api | `HTTP_ADDR` | `:8080` | адрес сервера |
| api | `DATABASE_URL` | `postgres://cosmoport:cosmoport@localhost:25432/cosmoport?sslmode=disable` | строка подключения к БД |
| api | `DB_MAX_CONNS` | `20` | размер пула соединений |
| api, mission-control | `RABBITMQ_URL` | `amqp://cosmoport:cosmoport@localhost:27672/` | брокер |
| api | `JWT_SECRET` | `change-me-in-production` | ключ подписи токенов |
| api | `JWT_TTL` | `24h` | срок жизни токена |
| api | `BCRYPT_COST` | `10` | сложность хэширования паролей |
| api | `SHUTDOWN_TIMEOUT` | `15s` | сколько ждать текущие запросы при остановке |
| mission-control | `WORKERS` | `50` (в compose 1000) | параллельно обрабатываемых заявок |
| mission-control | `MIN_STAGE_DELAY` / `MAX_STAGE_DELAY` | `1s` / `3s` | длительность этапа |
| mission-control | `REJECT_RATE` | `0.1` | доля отклонённых заявок |
| loadgen | `TARGET_URL`, `USERS`, `DURATION`, `RAMP_UP`, `THINK_MIN`, `THINK_MAX` | см. `cmd/loadgen/main.go` | параметры нагрузки |
| все | `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |

---

## 9. Структура проекта

```
.
├── cmd/                          # точки входа (main) — по одной на сервис
│   ├── api/main.go               # основной сервис
│   ├── mission-control/main.go   # сервис обработки (ЦУП)
│   └── loadgen/main.go           # нагрузочный сервис
├── internal/
│   ├── api/
│   │   ├── app/app.go            # сборка слоёв (DI), запуск и graceful shutdown
│   │   ├── domain/               # сущности: User, Order, статусы, ошибки
│   │   ├── handler/              # СЛОЙ 1: HTTP-хэндлеры + интерфейсы сервисов
│   │   ├── service/              # СЛОЙ 2: бизнес-логика + интерфейсы репозиториев
│   │   ├── repository/
│   │   │   ├── memory/           # СЛОЙ 3: приветствие для /test
│   │   │   └── postgres/         # СЛОЙ 3: PostgreSQL + migrations/*.sql
│   │   ├── middleware/           # JWT-авторизация, логи, метрики, request-id, recover
│   │   ├── auth/                 # выпуск и проверка JWT
│   │   ├── messaging/            # публикация событий и приём статусов (RabbitMQ)
│   │   ├── metrics/              # метрики Prometheus основного сервиса
│   │   └── web/                  # веб-интерфейс: static/index.html, app.js, style.css (go:embed)
│   ├── mission/                  # логика ЦУП: воркеры, этапы полёта, метрики
│   ├── loadgen/                  # сценарий виртуальных пользователей, метрики
│   ├── broker/                   # общая обёртка над RabbitMQ
│   ├── contracts/                # форматы сообщений между сервисами
│   ├── config/                   # чтение переменных окружения
│   └── logger/                   # JSON-логгер (slog)
├── deploy/
│   ├── prometheus/prometheus.yml
│   ├── grafana/provisioning/     # источники данных Prometheus и Loki, провайдер дашбордов
│   ├── grafana/dashboards/cosmoport.json
│   ├── promtail/config.yml
│   └── rabbitmq/                 # включение плагина метрик
├── scripts/demo.sh               # пошаговая демонстрация ЛР1–ЛР5
├── Dockerfile                    # один многоэтапный Dockerfile на 3 сервиса
├── docker-compose.yml
├── .env                          # внешние порты (меняй здесь, если порт занят)
├── Makefile
└── go.mod
```

---

## 10. Тесты

```bash
go test -race ./...      # или: make test
```

* `internal/api/auth/jwt_test.go`: выпуск и проверка токена; отказ для подделанного,
  просроченного и чужого токена.
* `internal/api/service/service_test.go`: регистрация, дубликаты, вход, валидация,
  создание заявки и публикация события, устойчивость к падению брокера, изоляция
  заявок разных пользователей, применение статусов. Тесты используют фейковые репозитории
  без БД, и это прямо показывает пользу изоляции через интерфейсы.
* `internal/api/handler/handler_test.go`: `/test` возвращает `Hello!` через настоящие
  слои service и repository, `/dbtest`, HTTP-коды ошибок, middleware передаёт ID
  пользователя из токена в хэндлер, без токена ответ `401`.

---

## 11. Сценарий защиты (что показывать преподавателю)

1. `docker compose up -d --build`, затем `docker compose ps`: все контейнеры запущены.
2. Открыть **http://localhost:28080**: зарегистрироваться, забронировать полёт и показать, как
   статус сам идёт до «Прибыли», а схема подсвечивает путь через RabbitMQ. Во вкладке
   «Проверка ЛР» нажать кнопки ЛР1, ЛР2 и ЛР5. В консоли внизу раскрыть запрос и показать
   заголовок `Authorization: Bearer …`. Тот же сценарий в терминале: `./scripts/demo.sh`.
3. Открыть код и показать 3 слоя: `handler/basic.go` → `service/greeting.go` →
   `repository/memory/greeting.go`, интерфейсы в `handler/handler.go` и `service/interfaces.go`.
4. RabbitMQ http://localhost:27673: две очереди с потребителями.
5. Prometheus http://localhost:29090/targets: все цели UP.
6. Запустить нагрузку `docker compose --profile load up -d loadgen` и открыть Grafana.
   Показать рост RPS, низкую задержку, 0% ошибок, поток заявок по статусам и логи внизу.
7. Под нагрузкой выполнить `docker compose stop mission-control`, затем
   `docker compose logs mission-control | tail`. В логе видно `returning to queue` и
   `stopped gracefully`, на дашборде растёт очередь. `docker compose start mission-control`:
   очередь разбирается, заявки доходят до `completed`.
8. `go test -race ./...`: тесты зелёные.

---

## 12. Вопросы, которые могут задать на защите

**Что такое чистая архитектура и зачем 3 слоя?**
Код разделён по ответственности: транспорт (HTTP), бизнес-логика и доступ к данным.
Слои общаются через интерфейсы, поэтому любой слой можно заменить, например PostgreSQL
на другую БД, не меняя остальные. Его также можно протестировать отдельно с подставными
объектами.

**Что такое graceful shutdown?**
Корректная остановка. По сигналу сервис перестаёт принимать новые запросы, дожидается
завершения текущих, дообрабатывает сообщение из очереди и закрывает соединения с БД и
брокером. Пользователи не получают оборванные ответы, данные не теряются.

**Почему пароли хранятся в bcrypt, а не в SHA-256?**
bcrypt специально медленный и использует соль. Подобрать пароль перебором по
утёкшему хэшу намного сложнее.

**Как устроен JWT?**
Это три части в base64: заголовок, данные (`sub` = ID пользователя, `exp` = срок
действия, `iss`) и подпись HMAC-SHA256 секретным ключом. Сервер не хранит сессии:
он проверяет подпись и срок, и если они верны, токену можно доверять.

**Почему в middleware проверяется алгоритм токена?**
Иначе злоумышленник может прислать токен с `alg: none` или с другим алгоритмом и
обойти проверку подписи. Мы явно разрешаем только HS256.

**Зачем RabbitMQ, почему не вызвать ЦУП по HTTP?**
Брокер развязывает сервисы. api не ждёт обработки заявки и отвечает клиенту сразу.
Если ЦУП упал, заявки копятся в очереди и будут обработаны после перезапуска.
Нагрузку можно распределить на несколько копий ЦУП.

**Что такое Ack/Nack и prefetch?**
`Ack` означает «сообщение обработано, удали его». `Nack` с requeue означает «верни в
очередь». `prefetch` ограничивает, сколько неподтверждённых сообщений брокер отдаёт
одному потребителю.

**Что будет, если одно сообщение придёт дважды?**
Статус может двигаться только вперёд, и это проверяется SQL-условием. Старое или
повторное событие просто игнорируется (идемпотентность).

**Чем Prometheus отличается от Loki?**
Prometheus хранит **числовые метрики** во времени (RPS, задержки, количество). Loki
хранит **текстовые логи**. Grafana показывает и то, и другое.

**Что такое p95?**
95% запросов выполнились быстрее этого значения. Перцентили лучше среднего показывают,
что ощущают пользователи.

**Почему метка `route`, а не путь запроса?**
Путь `/orders/123` уникален для каждой заявки. Это создало бы тысячи временных рядов
(«взрыв кардинальности»). Шаблон `GET /orders/{id}` один на все заявки.

---

## 13. Решение проблем

| Проблема | Решение |
|---|---|
| `port is already allocated` / `address already in use` | Порт занят другим приложением (например, сайтом). Поменяй нужный порт в файле `.env` (`API_PORT`, `GRAFANA_PORT`, `POSTGRES_PORT` и т.д.) и выполни `docker compose up -d`. Если поменял `API_PORT`, демо запускай так: `API_URL=http://localhost:<порт> ./scripts/demo.sh` |
| Ошибки скачивания образов (`EOF`, `failed to resolve`) | Временный сбой сети: повтори `docker compose up -d --build` |
| В Grafana пусто | Подожди 10–20 секунд: Prometheus опрашивает сервисы каждые 5 с. Для графиков нагрузки должен быть запущен loadgen |
| Нет логов в Grafana | Проверь `docker compose logs promtail`. Promtail нужен доступ к `/var/run/docker.sock` (в Docker Desktop он есть) |
| Заявки висят в `created` | Проверь, что запущен `mission-control`: `docker compose ps` |
| Начать с чистого листа | `docker compose --profile load down -v && docker compose up -d --build` |
