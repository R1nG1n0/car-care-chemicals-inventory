-- Схема garage v1.0 (ТЗ, раздел 3).
--
-- Время — целые секунды Unix (UTC), даты покупок — текст ГГГГ-ММ-ДД.
-- Количество — целое: мл или шт. Деньги — копейки.
--
-- Изоляция пользователей (ТБ-5) на уровне базы: у каждой таблицы с данными
-- есть user_id, а ссылки идут составными ключами (user_id, id). Сослаться
-- на категорию, место или позицию другого пользователя нельзя даже ошибкой
-- в коде — база откажет.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY,
    login         TEXT    NOT NULL UNIQUE COLLATE NOCASE,
    password_hash TEXT    NOT NULL,
    -- Владелец сайта: выдаёт приглашения (Р-1, Р-2).
    is_owner      INTEGER NOT NULL DEFAULT 0 CHECK (is_owner IN (0, 1)),
    created_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

-- Токены сессий и приглашений хранятся только хешами (ТБ-2).
CREATE TABLE sessions (
    token_hash   TEXT    PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
    last_seen_at INTEGER NOT NULL DEFAULT (unixepoch()),
    expires_at   INTEGER NOT NULL,
    user_agent   TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX sessions_user ON sessions (user_id);

CREATE TABLE invites (
    id         INTEGER PRIMARY KEY,
    token_hash TEXT    NOT NULL UNIQUE,
    created_by INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    expires_at INTEGER NOT NULL,
    used_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    used_at    INTEGER,
    revoked_at INTEGER
);

-- Шаг кнопок количества (ФТ-5.8).
CREATE TABLE user_settings (
    user_id    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    step_small INTEGER NOT NULL DEFAULT 10 CHECK (step_small BETWEEN 1 AND 1000),
    step_large INTEGER NOT NULL DEFAULT 50 CHECK (step_large BETWEEN 1 AND 5000)
);

-- ---------- Справочники (ФТ-2) ----------

CREATE TABLE categories (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    -- Только восемь цветов дизайн-системы.
    color    TEXT    NOT NULL CHECK (color IN ('blue', 'berry', 'olive', 'ochre', 'petrol', 'plum', 'slate', 'umber')),
    sort     INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    UNIQUE (user_id, name),
    UNIQUE (user_id, id)
);

CREATE TABLE locations (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    sort     INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    UNIQUE (user_id, name),
    UNIQUE (user_id, id)
);

CREATE TABLE stores (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    url      TEXT    NOT NULL DEFAULT '',
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    UNIQUE (user_id, name),
    UNIQUE (user_id, id)
);

-- ---------- Каталог (ФТ-3) ----------

CREATE TABLE products (
    id           INTEGER PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category_id  INTEGER NOT NULL,
    name         TEXT    NOT NULL,
    brand        TEXT    NOT NULL DEFAULT '',
    unit         TEXT    NOT NULL CHECK (unit IN ('ml', 'pcs')),
    package_size INTEGER NOT NULL CHECK (package_size > 0),
    barcode      TEXT    NOT NULL DEFAULT '',
    min_stock    INTEGER CHECK (min_stock IS NULL OR min_stock >= 0),
    note         TEXT    NOT NULL DEFAULT '',
    archived     INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    created_at   INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, category_id) REFERENCES categories (user_id, id)
);
CREATE INDEX products_user ON products (user_id, archived);
CREATE INDEX products_barcode ON products (user_id, barcode) WHERE barcode <> '';

-- ---------- Покупки (ФТ-4) ----------

CREATE TABLE purchases (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    store_id   INTEGER,
    date       TEXT    NOT NULL CHECK (date GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
    note       TEXT    NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, store_id) REFERENCES stores (user_id, id)
);
CREATE INDEX purchases_user ON purchases (user_id, date);

CREATE TABLE purchase_lines (
    id                INTEGER PRIMARY KEY,
    user_id           INTEGER NOT NULL,
    purchase_id       INTEGER NOT NULL,
    product_id        INTEGER NOT NULL,
    packages          INTEGER NOT NULL CHECK (packages > 0),
    price_per_package INTEGER NOT NULL CHECK (price_per_package >= 0),
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id, purchase_id) REFERENCES purchases (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id)
);
CREATE INDEX purchase_lines_product ON purchase_lines (user_id, product_id);

-- ---------- Экземпляры и движения (ФТ-5) ----------

CREATE TABLE units (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL,
    product_id       INTEGER NOT NULL,
    purchase_line_id INTEGER,
    location_id      INTEGER,
    initial_qty      INTEGER NOT NULL CHECK (initial_qty > 0),
    remaining_qty    INTEGER NOT NULL CHECK (remaining_qty >= 0 AND remaining_qty <= initial_qty),
    status           TEXT    NOT NULL DEFAULT 'sealed' CHECK (status IN ('sealed', 'open', 'finished', 'written_off')),
    opened_at        INTEGER,
    finished_at      INTEGER,
    created_at       INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, id),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id),
    FOREIGN KEY (user_id, purchase_line_id) REFERENCES purchase_lines (user_id, id),
    FOREIGN KEY (user_id, location_id) REFERENCES locations (user_id, id)
);
CREATE INDEX units_product ON units (user_id, product_id, status);
CREATE INDEX units_location ON units (user_id, location_id);

-- Применение шаблона расхода: одна запись на нажатие «Списать всё».
CREATE TABLE recipe_runs (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    recipe_id  INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, id)
);

CREATE TABLE movements (
    id               INTEGER PRIMARY KEY,
    user_id          INTEGER NOT NULL,
    unit_id          INTEGER NOT NULL,
    type             TEXT    NOT NULL CHECK (type IN ('purchase', 'consume', 'writeoff', 'move', 'adjust')),
    -- Изменение остатка экземпляра: приход > 0, расход и списание < 0, перемещение 0.
    qty_delta        INTEGER NOT NULL,
    from_location_id INTEGER,
    to_location_id   INTEGER,
    reason           TEXT    NOT NULL DEFAULT '',
    recipe_run_id    INTEGER,
    created_at       INTEGER NOT NULL DEFAULT (unixepoch()),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, unit_id) REFERENCES units (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, from_location_id) REFERENCES locations (user_id, id),
    FOREIGN KEY (user_id, to_location_id) REFERENCES locations (user_id, id),
    FOREIGN KEY (user_id, recipe_run_id) REFERENCES recipe_runs (user_id, id)
);
CREATE INDEX movements_unit ON movements (user_id, unit_id, created_at);
CREATE INDEX movements_time ON movements (user_id, created_at);

-- ---------- Список покупок (ФТ-7) ----------

-- Позиции «ниже минимума» вычисляются запросом; здесь — добавленные вручную:
-- позиция каталога или просто текст («Глина для кузова»).
CREATE TABLE shopping_items (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    product_id INTEGER,
    title      TEXT    NOT NULL DEFAULT '',
    done_at    INTEGER,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    CHECK (product_id IS NOT NULL OR title <> ''),
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE
);
CREATE INDEX shopping_items_user ON shopping_items (user_id, done_at);

-- ---------- Шаблоны расхода (ФТ-8) ----------

CREATE TABLE recipes (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    sort       INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    UNIQUE (user_id, name),
    UNIQUE (user_id, id)
);

CREATE TABLE recipe_items (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    recipe_id  INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    qty        INTEGER NOT NULL CHECK (qty > 0),
    sort       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id, recipe_id) REFERENCES recipes (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id)
);
CREATE INDEX recipe_items_recipe ON recipe_items (user_id, recipe_id, sort);
