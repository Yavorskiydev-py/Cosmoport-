-- ЛР2: тестовая таблица для эндпоинта POST /dbtest
CREATE TABLE IF NOT EXISTS test_messages (
    id         BIGSERIAL PRIMARY KEY,
    text       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ЛР3: пользователи (космические путешественники)
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    login         VARCHAR(32) NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ЛР4: заявки на космические полёты
CREATE TABLE IF NOT EXISTS orders (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    destination   VARCHAR(32) NOT NULL,
    passengers    INT         NOT NULL CHECK (passengers > 0),
    price         BIGINT      NOT NULL CHECK (price >= 0),
    status        VARCHAR(16) NOT NULL DEFAULT 'created',
    status_reason TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders (status);
