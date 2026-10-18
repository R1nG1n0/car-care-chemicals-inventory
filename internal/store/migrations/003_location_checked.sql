-- Инвентаризация (ФТ-10.1.4): когда место последний раз проверяли.
-- unix-секунды; NULL — место ещё не проверялось.
ALTER TABLE locations ADD COLUMN checked_at INTEGER;
