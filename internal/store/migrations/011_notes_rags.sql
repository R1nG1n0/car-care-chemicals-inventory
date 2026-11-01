-- v1.7: заметки (ФТ-18) и ресурс микрофибр и губок (ФТ-19).
--
-- notes — заметки пользователя; note_products и note_recipes — связи с
-- позициями и шаблонами. Составные ключи не дают связать заметку с чужой
-- позицией или шаблоном, удаление любой стороны убирает только связь.
CREATE TABLE notes (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title      TEXT    NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    body       TEXT    NOT NULL DEFAULT '' CHECK (length(body) <= 10000),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (user_id, id)
);
CREATE INDEX notes_user_updated ON notes (user_id, updated_at DESC, id DESC);

CREATE TABLE note_products (
    user_id    INTEGER NOT NULL,
    note_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, note_id, product_id),
    FOREIGN KEY (user_id, note_id) REFERENCES notes (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE
);
CREATE INDEX note_products_product ON note_products (user_id, product_id);

CREATE TABLE note_recipes (
    user_id   INTEGER NOT NULL,
    note_id   INTEGER NOT NULL,
    recipe_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, note_id, recipe_id),
    FOREIGN KEY (user_id, note_id) REFERENCES notes (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, recipe_id) REFERENCES recipes (user_id, id) ON DELETE CASCADE
);
CREATE INDEX note_recipes_recipe ON note_recipes (user_id, recipe_id);

-- Ресурс позиции в шт: сколько стирок выдерживает одна тряпка; NULL — учёта нет.
ALTER TABLE products ADD COLUMN wear_limit INTEGER
    CHECK (wear_limit IS NULL OR (wear_limit BETWEEN 1 AND 1000));

-- rags — тряпки в работе. number — порядковый номер по позиции (метка по
-- умолчанию «№ N»), consume_movement_id — движение расхода при «В работу».
CREATE TABLE rags (
    id                  INTEGER PRIMARY KEY,
    user_id             INTEGER NOT NULL,
    product_id          INTEGER NOT NULL,
    label               TEXT    NOT NULL CHECK (length(label) BETWEEN 1 AND 20),
    number              INTEGER NOT NULL CHECK (number >= 1),
    washes              INTEGER NOT NULL DEFAULT 0 CHECK (washes >= 0),
    status              TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'retired')),
    retire_reason       TEXT    CHECK (retire_reason IS NULL OR retire_reason IN ('worn', 'lost', 'damaged')),
    consume_movement_id INTEGER,
    created_at          INTEGER NOT NULL,
    retired_at          INTEGER,
    last_washed_at      INTEGER,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE,
    UNIQUE (user_id, id)
);
CREATE INDEX rags_product ON rags (user_id, product_id, status, number);
