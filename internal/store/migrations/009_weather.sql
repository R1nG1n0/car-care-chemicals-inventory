-- v1.8 (ФТ-16): своё место для погоды. Таблица служебная и в экспорт/импорт
-- не входит. Нет строки — действует место по умолчанию из конфигурации.
CREATE TABLE weather_places (
    user_id INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    name    TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 320),
    lat     REAL NOT NULL CHECK (lat BETWEEN -90 AND 90),
    lon     REAL NOT NULL CHECK (lon BETWEEN -180 AND 180)
);
