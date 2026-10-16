-- Группа движений одного запроса (одного нажатия «Списать», одной покупки,
-- одного применения шаблона): batch_id = id первого движения запроса.
-- По ней «последний расход» (last_consume_qty) суммирует ровно одно нажатие,
-- даже если два расхода пришлись на одну секунду.
ALTER TABLE movements ADD COLUMN batch_id INTEGER;

-- До миграции группы не было: каждое старое движение — своя группа.
UPDATE movements SET batch_id = id;

CREATE INDEX movements_batch ON movements (user_id, batch_id);
