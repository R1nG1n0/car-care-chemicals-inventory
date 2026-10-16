package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PurchaseInput — тело POST /api/purchases.
type PurchaseInput struct {
	Date    string              `json:"date"`
	StoreID *int64              `json:"store_id"`
	Note    string              `json:"note"`
	Lines   []PurchaseLineInput `json:"lines"`
}

// PurchaseLineInput — строка покупки.
type PurchaseLineInput struct {
	ProductID       int64  `json:"product_id"`
	Packages        int64  `json:"packages"`
	PricePerPackage int64  `json:"price_per_package"`
	LocationID      *int64 `json:"location_id"`
}

// Purchase — покупка в формате API.
type Purchase struct {
	ID        int64          `json:"id"`
	Date      string         `json:"date"`
	StoreID   *int64         `json:"store_id"`
	Store     *string        `json:"store"` // null — без магазина
	Note      string         `json:"note"`
	Total     int64          `json:"total"`
	Lines     []PurchaseLine `json:"lines"`
	CreatedAt int64          `json:"created_at"`
}

// PurchaseLine — строка покупки с созданными экземплярами.
type PurchaseLine struct {
	ID              int64   `json:"id"`
	ProductID       int64   `json:"product_id"`
	Product         string  `json:"product"`
	Packages        int64   `json:"packages"`
	PricePerPackage int64   `json:"price_per_package"`
	UnitIDs         []int64 `json:"unit_ids"`
}

// PurchaseShort — строка списка покупок.
type PurchaseShort struct {
	ID         int64   `json:"id"`
	Date       string  `json:"date"`
	Store      *string `json:"store"`
	Total      int64   `json:"total"`
	LinesCount int64   `json:"lines_count"`
}

func validatePurchase(in *PurchaseInput) error {
	in.Date = strings.TrimSpace(in.Date)
	if _, err := time.Parse("2006-01-02", in.Date); err != nil {
		return &ValidationError{Field: "date", Message: "Дата — в виде ГГГГ-ММ-ДД"}
	}
	note, err := cleanText(in.Note, 0, maxNoteLen, "note", "Заметка — не длиннее 500 символов")
	if err != nil {
		return err
	}
	in.Note = note
	if len(in.Lines) < 1 || len(in.Lines) > maxLines {
		return &ValidationError{Field: "lines", Message: "В покупке — от 1 до 30 строк"}
	}
	for _, l := range in.Lines {
		if l.Packages < 1 || l.Packages > maxPackages {
			return &ValidationError{Field: "packages", Message: "Количество упаковок — от 1 до 100"}
		}
		if l.PricePerPackage < 0 || l.PricePerPackage > maxPrice {
			return &ValidationError{Field: "price_per_package", Message: "Цена — от 0 до 1 000 000 ₽"}
		}
	}
	return nil
}

// CreatePurchase записывает покупку одной транзакцией: покупка, строки, по
// экземпляру на каждую упаковку (sealed, полный остаток), движение purchase
// на каждый экземпляр; невыполненные пункты списка покупок с этими
// позициями отмечаются выполненными (ФТ-4, ФТ-7).
func (s *Service) CreatePurchase(ctx context.Context, userID int64, in PurchaseInput) (int64, error) {
	if err := validatePurchase(&in); err != nil {
		return 0, err
	}
	now := s.now().Unix()
	var purchaseID int64
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		var store sql.NullInt64
		if in.StoreID != nil {
			var one int
			err := tx.QueryRowContext(ctx, `SELECT 1 FROM stores WHERE user_id = ? AND id = ?`,
				userID, *in.StoreID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return errStoreNotFound
			}
			if err != nil {
				return fmt.Errorf("чтение магазина: %w", err)
			}
			store = sql.NullInt64{Int64: *in.StoreID, Valid: true}
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO purchases (user_id, store_id, date, note, created_at) VALUES (?, ?, ?, ?, ?)`,
			userID, store, in.Date, in.Note, now)
		if err != nil {
			return fmt.Errorf("запись покупки: %w", err)
		}
		if purchaseID, err = res.LastInsertId(); err != nil {
			return err
		}
		for _, l := range in.Lines {
			if err := insertLine(ctx, tx, userID, purchaseID, l, now); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `
			UPDATE shopping_items SET done_at = ?
			WHERE user_id = ? AND done_at IS NULL AND product_id IN
			      (SELECT product_id FROM purchase_lines WHERE user_id = ? AND purchase_id = ?)`,
			now, userID, userID, purchaseID)
		if err != nil {
			return fmt.Errorf("отметка списка покупок: %w", err)
		}
		return nil
	})
	return purchaseID, err
}

func insertLine(ctx context.Context, tx *sql.Tx, userID, purchaseID int64, l PurchaseLineInput, now int64) error {
	p, err := product(ctx, tx, userID, l.ProductID)
	if err != nil {
		return err
	}
	loc, err := resolveLocation(ctx, tx, userID, p.ID, l.LocationID)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO purchase_lines (user_id, purchase_id, product_id, packages, price_per_package)
		VALUES (?, ?, ?, ?, ?)`, userID, purchaseID, p.ID, l.Packages, l.PricePerPackage)
	if err != nil {
		return fmt.Errorf("запись строки покупки: %w", err)
	}
	lineID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for range l.Packages {
		res, err := tx.ExecContext(ctx, `
			INSERT INTO units (user_id, product_id, purchase_line_id, location_id, initial_qty,
			                   remaining_qty, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'sealed', ?)`,
			userID, p.ID, lineID, loc, p.PackageSize, p.PackageSize, now)
		if err != nil {
			return fmt.Errorf("запись экземпляра: %w", err)
		}
		unitID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if _, err := insertMovement(ctx, tx, movementRow{
			userID: userID, unitID: unitID, typ: "purchase", delta: p.PackageSize,
			to: loc, at: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Purchase читает свою покупку со строками и экземплярами.
func (s *Service) Purchase(ctx context.Context, userID, id int64) (Purchase, error) {
	p := Purchase{ID: id, Lines: []PurchaseLine{}}
	var store sql.NullInt64
	var storeName sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT p.date, p.store_id, s.name, p.note, p.created_at FROM purchases p
		LEFT JOIN stores s ON s.user_id = p.user_id AND s.id = p.store_id
		WHERE p.user_id = ? AND p.id = ?`, userID, id).
		Scan(&p.Date, &store, &storeName, &p.Note, &p.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, errPurchaseNotFound
	}
	if err != nil {
		return p, fmt.Errorf("чтение покупки: %w", err)
	}
	if store.Valid {
		p.StoreID = &store.Int64
	}
	if storeName.Valid {
		p.Store = &storeName.String
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT l.id, l.product_id, pr.name, l.packages, l.price_per_package FROM purchase_lines l
		JOIN products pr ON pr.user_id = l.user_id AND pr.id = l.product_id
		WHERE l.user_id = ? AND l.purchase_id = ? ORDER BY l.id`, userID, id)
	if err != nil {
		return p, fmt.Errorf("чтение строк покупки: %w", err)
	}
	defer rows.Close()
	index := map[int64]int{}
	for rows.Next() {
		l := PurchaseLine{UnitIDs: []int64{}}
		if err := rows.Scan(&l.ID, &l.ProductID, &l.Product, &l.Packages, &l.PricePerPackage); err != nil {
			return p, err
		}
		p.Total += l.Packages * l.PricePerPackage
		index[l.ID] = len(p.Lines)
		p.Lines = append(p.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return p, err
	}
	rows.Close()

	urows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.purchase_line_id FROM units u
		JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id
		WHERE u.user_id = ? AND l.purchase_id = ? ORDER BY u.id`, userID, id)
	if err != nil {
		return p, fmt.Errorf("чтение экземпляров покупки: %w", err)
	}
	defer urows.Close()
	for urows.Next() {
		var unitID, lineID int64
		if err := urows.Scan(&unitID, &lineID); err != nil {
			return p, err
		}
		if i, ok := index[lineID]; ok {
			p.Lines[i].UnitIDs = append(p.Lines[i].UnitIDs, unitID)
		}
	}
	return p, urows.Err()
}

// Purchases — список покупок, новые сверху; before — id, с которого
// начинается следующая страница (0 — с начала).
func (s *Service) Purchases(ctx context.Context, userID, before int64, limit int) ([]PurchaseShort, error) {
	limit, err := validLimit(limit)
	if err != nil {
		return nil, err
	}
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.date, s.name,
		       COALESCE((SELECT SUM(packages * price_per_package) FROM purchase_lines
		                 WHERE user_id = p.user_id AND purchase_id = p.id), 0),
		       (SELECT COUNT(*) FROM purchase_lines WHERE user_id = p.user_id AND purchase_id = p.id)
		FROM purchases p
		LEFT JOIN stores s ON s.user_id = p.user_id AND s.id = p.store_id
		WHERE p.user_id = ? AND p.id < ?
		ORDER BY p.id DESC LIMIT ?`, userID, before, limit)
	if err != nil {
		return nil, fmt.Errorf("список покупок: %w", err)
	}
	defer rows.Close()
	out := []PurchaseShort{}
	for rows.Next() {
		var p PurchaseShort
		var store sql.NullString
		if err := rows.Scan(&p.ID, &p.Date, &store, &p.Total, &p.LinesCount); err != nil {
			return nil, err
		}
		if store.Valid {
			p.Store = &store.String
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
