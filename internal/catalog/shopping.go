package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// LowItem — позиция ниже минимума в списке покупок.
type LowItem struct {
	Product       ProductSummary `json:"product"`
	LastPrice     *int64         `json:"last_price"`
	LastStore     *string        `json:"last_store"`
	CheapestStore *string        `json:"cheapest_store"`
}

// ShoppingItem — пункт, добавленный вручную.
type ShoppingItem struct {
	ID        int64           `json:"id"`
	ProductID *int64          `json:"product_id"`
	Title     string          `json:"title"`
	Product   *ProductSummary `json:"product"`
	DoneAt    *int64          `json:"done_at"`
	CreatedAt int64           `json:"created_at"`
}

// ShoppingList — ответ GET /api/shopping.
type ShoppingList struct {
	Low    []LowItem      `json:"low"`
	Manual []ShoppingItem `json:"manual"`
}

// shoppingDoneWindow — сколько показываются выполненные пункты (7 дней).
const shoppingDoneWindow = 7 * 24 * 60 * 60

// Shopping — список покупок на момент now (секунды Unix). Только чтение:
// купленные пункты отмечает транзакция покупки (stock.CreatePurchase, ФТ-7).
func (s *Service) Shopping(ctx context.Context, userID, now int64) (ShoppingList, error) {
	list := ShoppingList{Low: []LowItem{}, Manual: []ShoppingItem{}}

	low, err := s.Products(ctx, userID, ProductFilter{Filter: FilterLow})
	if err != nil {
		return ShoppingList{}, err
	}
	for _, p := range low {
		item, err := s.lowItem(ctx, userID, p)
		if err != nil {
			return ShoppingList{}, err
		}
		list.Low = append(list.Low, item)
	}

	rows, err := s.q.QueryContext(ctx, `SELECT id FROM shopping_items
		WHERE user_id = ? AND (done_at IS NULL OR done_at >= ?) ORDER BY created_at DESC, id DESC`,
		userID, now-shoppingDoneWindow)
	if err != nil {
		return ShoppingList{}, fmt.Errorf("список покупок: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return ShoppingList{}, fmt.Errorf("список покупок: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ShoppingList{}, fmt.Errorf("список покупок: %w", err)
	}
	for _, id := range ids {
		it, err := s.shoppingItem(ctx, userID, id)
		if err != nil {
			return ShoppingList{}, err
		}
		list.Manual = append(list.Manual, it)
	}
	return list, nil
}

// lowItem — последняя цена и магазин позиции и магазин, где дешевле всего
// за 100 мл / за штуку (по объёму упаковки строки, см. lineSizeExpr).
func (s *Service) lowItem(ctx context.Context, userID int64, p ProductSummary) (LowItem, error) {
	item := LowItem{Product: p}
	var price sql.NullInt64
	var store sql.NullString
	err := s.q.QueryRowContext(ctx, `SELECT l.price_per_package, st.name
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		LEFT JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE l.user_id = ? AND l.product_id = ?
		ORDER BY pu.date DESC, pu.id DESC, l.id DESC LIMIT 1`, userID, p.ID).Scan(&price, &store)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LowItem{}, fmt.Errorf("последняя цена: %w", err)
	}
	item.LastPrice, item.LastStore = nullInt(price), nullString(store)

	var cheapest sql.NullString
	err = s.q.QueryRowContext(ctx, `SELECT st.name
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		JOIN products pr ON pr.user_id = l.user_id AND pr.id = l.product_id
		JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE l.user_id = ? AND l.product_id = ?
		ORDER BY CAST(l.price_per_package AS REAL) / `+lineSizeExpr+`, pu.date DESC, pu.id DESC LIMIT 1`,
		userID, p.ID).Scan(&cheapest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return LowItem{}, fmt.Errorf("дешёвый магазин: %w", err)
	}
	item.CheapestStore = nullString(cheapest)
	return item, nil
}

func (s *Service) shoppingItem(ctx context.Context, userID, id int64) (ShoppingItem, error) {
	var it ShoppingItem
	var product, done sql.NullInt64
	err := s.q.QueryRowContext(ctx, `SELECT id, product_id, title, done_at, created_at
		FROM shopping_items WHERE user_id = ? AND id = ?`, userID, id).
		Scan(&it.ID, &product, &it.Title, &done, &it.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ShoppingItem{}, notFound(msgShoppingNotFound)
	}
	if err != nil {
		return ShoppingItem{}, fmt.Errorf("пункт списка покупок: %w", err)
	}
	it.ProductID, it.DoneAt = nullInt(product), nullInt(done)
	if it.ProductID != nil {
		p, err := s.Summary(ctx, userID, *it.ProductID)
		if err != nil {
			return ShoppingItem{}, err
		}
		it.Product = &p
	}
	return it, nil
}

// AddShopping добавляет пункт: позицию (своя, иначе 404) и/или текст 1–80.
func (s *Service) AddShopping(ctx context.Context, userID int64, productID *int64, title string, at int64) (ShoppingItem, error) {
	title, err := optText("title", title, 80, "Название")
	if err != nil {
		return ShoppingItem{}, err
	}
	if productID == nil && title == "" {
		return ShoppingItem{}, invalid("title", "Укажите позицию или название")
	}
	if productID != nil {
		if _, err := s.Summary(ctx, userID, *productID); err != nil {
			return ShoppingItem{}, err
		}
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO shopping_items (user_id, product_id, title, created_at)
		VALUES (?, ?, ?, ?)`, userID, productID, title, at)
	if err != nil {
		return ShoppingItem{}, fmt.Errorf("пункт списка покупок: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.shoppingItem(ctx, userID, id)
}

// SetShoppingDone отмечает пункт выполненным или снимает отметку. Снятый
// пункт поднимается наверх списка: created_at = at.
func (s *Service) SetShoppingDone(ctx context.Context, userID, id int64, done bool, at int64) (ShoppingItem, error) {
	if _, err := s.shoppingItem(ctx, userID, id); err != nil {
		return ShoppingItem{}, err
	}
	var err error
	if done {
		_, err = s.q.ExecContext(ctx, `UPDATE shopping_items SET done_at = ? WHERE user_id = ? AND id = ?`, at, userID, id)
	} else {
		_, err = s.q.ExecContext(ctx, `UPDATE shopping_items SET done_at = NULL, created_at = ?
			WHERE user_id = ? AND id = ?`, at, userID, id)
	}
	if err != nil {
		return ShoppingItem{}, fmt.Errorf("пункт списка покупок: %w", err)
	}
	return s.shoppingItem(ctx, userID, id)
}

// DeleteShopping удаляет пункт.
func (s *Service) DeleteShopping(ctx context.Context, userID, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM shopping_items WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("пункт списка покупок: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound(msgShoppingNotFound)
	}
	return nil
}
