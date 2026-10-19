-- Telegram-бот (ФТ-11). Таблицы служебные и в экспорт/импорт не входят.
--
-- telegram_links: связь пользователя сайта с Telegram-аккаунтом. Один
-- пользователь — одна привязка (PK), один Telegram-аккаунт — один
-- пользователь (UNIQUE tg_user_id). Новая привязка заменяет старую (ФТ-11.2.2).
CREATE TABLE telegram_links (
    user_id    INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    tg_user_id INTEGER NOT NULL UNIQUE,
    chat_id    INTEGER NOT NULL,
    tg_name    TEXT    NOT NULL DEFAULT '',
    linked_at  INTEGER NOT NULL
);

-- telegram_link_codes: одноразовые коды привязки. В базе только sha256-хеш
-- кода (ФТ-11.2.1); срок — 10 минут.
CREATE TABLE telegram_link_codes (
    hash       TEXT    PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL
);
CREATE INDEX telegram_link_codes_user_idx ON telegram_link_codes (user_id);

-- bot_state: состояние бота (номер последнего обработанного обновления, ФТ-11.1.4).
CREATE TABLE bot_state (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- bot_low_state: позиции, которые уже «ниже минимума» и о которых сообщено
-- (ФТ-11.7.2). Позиция вышла из «низких» — строка удаляется, и следующее
-- падение снова вызовет уведомление.
CREATE TABLE bot_low_state (
    user_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    PRIMARY KEY (user_id, product_id),
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE
);
