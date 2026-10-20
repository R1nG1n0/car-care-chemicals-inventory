-- Идемпотентность расхода без связи (ФТ-12.4.1). Таблица служебная и в
-- экспорт/импорт не входит.
--
-- client_ops: номер операции, присланный телефоном, и сохранённый ответ.
-- Запись делается в одной транзакции со списанием: повтор с тем же номером
-- возвращает прежний ответ и ничего не списывает. Записи старше 90 дней
-- удаляются при вставке (индекс по created_at).
CREATE TABLE client_ops (
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    op_id      TEXT    NOT NULL,
    kind       TEXT    NOT NULL,
    response   TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, op_id)
);
CREATE INDEX client_ops_created_idx ON client_ops (created_at);
