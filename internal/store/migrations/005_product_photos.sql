-- Фото позиции (ФТ-10.4): одно на позицию, хранится в базе, поэтому входит
-- в ежедневный снимок и ночную копию. Составной ключ и каскад — как у
-- остальных таблиц: фото нельзя привязать к чужой позиции, а при удалении
-- позиции или пользователя оно уходит вместе с ней.
--
-- Предел 400 КБ и 4096 px продублирован в internal/photo; браузер сжимает
-- фото до 1024 px и 300 КБ заранее. updated_at растёт при каждой замене
-- (даже в ту же секунду) — по нему клиент строит адрес ?v= для кэша.
CREATE TABLE product_photos (
    user_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    mime       TEXT    NOT NULL CHECK (mime IN ('image/jpeg', 'image/png', 'image/webp')),
    data       BLOB    NOT NULL CHECK (length(data) BETWEEN 1 AND 409600),
    width      INTEGER NOT NULL CHECK (width BETWEEN 1 AND 4096),
    height     INTEGER NOT NULL CHECK (height BETWEEN 1 AND 4096),
    updated_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, product_id),
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE
);
