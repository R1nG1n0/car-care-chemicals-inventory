-- v1.6 (ФТ-17): разбавление концентратов.
--
-- product_dilutions — пропорции разбавления позиции (название и 1:N);
-- составной ключ не даёт привязать пропорцию к чужой позиции, удаление
-- позиции убирает её пропорции.
CREATE TABLE product_dilutions (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER NOT NULL,
    product_id INTEGER NOT NULL,
    name       TEXT    NOT NULL CHECK (length(name) BETWEEN 1 AND 40),
    ratio      INTEGER NOT NULL CHECK (ratio BETWEEN 1 AND 1000),
    sort       INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (user_id, product_id) REFERENCES products (user_id, id) ON DELETE CASCADE
);
CREATE INDEX product_dilutions_product ON product_dilutions (user_id, product_id, sort);

-- Раствор, из которого посчитан расход: объём готового раствора (мл) и
-- пропорция 1:N. qty_delta / qty хранят рассчитанный концентрат.
ALTER TABLE movements ADD COLUMN solution_ml INTEGER CHECK (solution_ml IS NULL OR (solution_ml BETWEEN 1 AND 100000));
ALTER TABLE movements ADD COLUMN solution_ratio INTEGER CHECK (solution_ratio IS NULL OR (solution_ratio BETWEEN 1 AND 1000));
ALTER TABLE recipe_items ADD COLUMN solution_ml INTEGER CHECK (solution_ml IS NULL OR (solution_ml BETWEEN 1 AND 100000));
ALTER TABLE recipe_items ADD COLUMN solution_ratio INTEGER CHECK (solution_ratio IS NULL OR (solution_ratio BETWEEN 1 AND 1000));
