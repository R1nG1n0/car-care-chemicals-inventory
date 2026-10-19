-- Отчёты о тратах и каскадное удаление идут по (user_id, purchase_id).
CREATE INDEX IF NOT EXISTS purchase_lines_purchase ON purchase_lines (user_id, purchase_id);
