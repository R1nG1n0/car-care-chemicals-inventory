package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// lastPurchase — последняя покупка позиции: цена за упаковку, магазин.
type lastPurchase struct {
	price     int64
	storeID   *int64
	storeName string
}

// lastPurchaseOf читает последнюю покупку позиции пользователя; nil — покупок не было.
func (b *Bot) lastPurchaseOf(ctx context.Context, userID, productID int64) (*lastPurchase, error) {
	var price int64
	var storeID sql.NullInt64
	var storeName sql.NullString
	err := b.db.QueryRowContext(ctx, `
		SELECT l.price_per_package, pu.store_id, st.name
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		LEFT JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE l.user_id = ? AND l.product_id = ?
		ORDER BY pu.date DESC, pu.id DESC, l.id DESC LIMIT 1`, userID, productID).
		Scan(&price, &storeID, &storeName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("последняя покупка: %w", err)
	}
	lp := &lastPurchase{price: price, storeName: storeName.String}
	if storeID.Valid {
		lp.storeID = &storeID.Int64
	}
	return lp, nil
}
