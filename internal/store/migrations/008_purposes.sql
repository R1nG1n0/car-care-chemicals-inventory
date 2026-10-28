-- v1.5 (ФТ-15): назначения средств, свойства позиции, оценка.
--
-- purposes — свой справочник пользователя, как categories (без цвета).
-- product_purposes — связь «позиция — назначение»; составные ключи не дают
-- связать позицию с чужим назначением, удаление позиции или назначения
-- убирает связь.
CREATE TABLE purposes (
    id       INTEGER PRIMARY KEY,
    user_id  INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name     TEXT    NOT NULL,
    sort     INTEGER NOT NULL DEFAULT 0,
    archived INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
    UNIQUE (user_id, name),
    UNIQUE (user_id, id)
);

CREATE TABLE product_purposes (
    user_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    purpose_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, product_id, purpose_id),
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE,
    FOREIGN KEY (user_id, purpose_id) REFERENCES purposes (user_id, id) ON DELETE CASCADE
);
CREATE INDEX product_purposes_purpose ON product_purposes (user_id, purpose_id);

-- pH хранится в десятых долях (73 = 7,3).
ALTER TABLE products ADD COLUMN ph_x10 INTEGER CHECK (ph_x10 IS NULL OR (ph_x10 BETWEEN 0 AND 140));
ALTER TABLE products ADD COLUMN incompatible TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN instruction TEXT NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN rating INTEGER CHECK (rating IS NULL OR (rating BETWEEN 1 AND 5));
ALTER TABLE products ADD COLUMN do_not_buy INTEGER NOT NULL DEFAULT 0 CHECK (do_not_buy IN (0, 1));

-- Стартовый набор назначений существующим пользователям (ФТ-15.1.2);
-- новым он засевается при создании пользователя (internal/auth).
INSERT INTO purposes (user_id, name, sort)
SELECT u.id, v.name, v.sort
FROM users u
CROSS JOIN (
    SELECT 'ЛКП' AS name, 1 AS sort
    UNION ALL SELECT 'стекло', 2
    UNION ALL SELECT 'пластик', 3
    UNION ALL SELECT 'кожа', 4
    UNION ALL SELECT 'диски', 5
    UNION ALL SELECT 'шины', 6
    UNION ALL SELECT 'текстиль', 7
) v;
