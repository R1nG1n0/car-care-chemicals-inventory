-- NULL не сохраняет границы запроса: безопасно восстановить только одиночные группы.
-- У известного ключа сохраняем группу внутри пользователя. Проверка self-anchor
-- отличает живую группу от ссылки на переиспользованный SQLite id, если новая
-- запись принадлежит другой группе или пользователю. Если id и self-anchor
-- совпали после переиспользования, исторических данных для различения уже нет.
WITH repairs AS MATERIALIZED (
    SELECT m.user_id, m.batch_id, MIN(m.id) AS first_id
    FROM movements m
    WHERE m.batch_id IS NOT NULL
      AND NOT EXISTS (
          SELECT 1 FROM movements anchor
          WHERE anchor.user_id = m.user_id AND anchor.id = m.batch_id
            AND anchor.batch_id = anchor.id
      )
    GROUP BY m.user_id, m.batch_id
)
UPDATE movements
SET batch_id = (SELECT first_id FROM repairs
               WHERE repairs.user_id = movements.user_id
                 AND repairs.batch_id = movements.batch_id)
WHERE EXISTS (SELECT 1 FROM repairs
              WHERE repairs.user_id = movements.user_id
                AND repairs.batch_id = movements.batch_id);

UPDATE movements SET batch_id = id WHERE batch_id IS NULL;
