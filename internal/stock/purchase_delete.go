package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// errPurchaseUsed — экземпляры покупки уже тронуты, удалить её целиком нельзя.
var errPurchaseUsed = &ConflictError{Message: "Экземпляры покупки уже использованы — отменить нельзя"}

// DeletePurchase удаляет свою покупку целиком, пока все её экземпляры не
// тронуты (ФТ-11.6.4): запечатаны, остаток равен объёму и в журнале у них
// только движение purchase. Иначе — ConflictError, ничего не меняется.
// Одна транзакция: движения, экземпляры, строки, покупка. Ручные пункты
// списка покупок, вычеркнутые этой покупкой, возвращаются. HTTP-адреса у
// метода нет — им пользуется только бот.
func (s *Service) DeletePurchase(ctx context.Context, userID, purchaseID int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var createdAt int64
		err := tx.QueryRowContext(ctx, `SELECT created_at FROM purchases WHERE user_id = ? AND id = ?`,
			userID, purchaseID).Scan(&createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			return errPurchaseNotFound
		}
		if err != nil {
			return fmt.Errorf("чтение покупки: %w", err)
		}

		// Тронутые экземпляры: не запечатаны, остаток не равен объёму
		// или есть движения кроме ввода покупки.
		var touched int64
		err = tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM units u
			JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id
			WHERE u.user_id = ? AND l.purchase_id = ?
			  AND (u.status <> 'sealed' OR u.remaining_qty <> u.initial_qty
			       OR EXISTS (SELECT 1 FROM movements m
			                  WHERE m.user_id = u.user_id AND m.unit_id = u.id AND m.type <> 'purchase'))`,
			userID, purchaseID).Scan(&touched)
		if err != nil {
			return fmt.Errorf("проверка экземпляров покупки: %w", err)
		}
		if touched > 0 {
			return errPurchaseUsed
		}

		const unitsOfPurchase = `(SELECT u.id FROM units u
			JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id
			WHERE u.user_id = ? AND l.purchase_id = ?)`
		// Обратное к CreatePurchase: пункты списка покупок с позициями покупки,
		// вычеркнутые в момент её создания, снова становятся невыполненными.
		if _, err := tx.ExecContext(ctx, `
			UPDATE shopping_items SET done_at = NULL
			WHERE user_id = ? AND done_at = ? AND product_id IN
			      (SELECT product_id FROM purchase_lines WHERE user_id = ? AND purchase_id = ?)`,
			userID, createdAt, userID, purchaseID); err != nil {
			return fmt.Errorf("возврат отметок списка покупок: %w", err)
		}
		steps := []struct{ what, q string }{
			{"движений", `DELETE FROM movements WHERE user_id = ? AND unit_id IN ` + unitsOfPurchase},
			{"экземпляров", `DELETE FROM units WHERE user_id = ? AND id IN ` + unitsOfPurchase},
		}
		for _, st := range steps {
			if _, err := tx.ExecContext(ctx, st.q, userID, userID, purchaseID); err != nil {
				return fmt.Errorf("удаление %s покупки: %w", st.what, err)
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM purchase_lines WHERE user_id = ? AND purchase_id = ?`,
			userID, purchaseID); err != nil {
			return fmt.Errorf("удаление строк покупки: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM purchases WHERE user_id = ? AND id = ?`,
			userID, purchaseID); err != nil {
			return fmt.Errorf("удаление покупки: %w", err)
		}
		return nil
	})
}
