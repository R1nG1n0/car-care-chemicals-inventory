package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

// Stock — остаток позиции по экземплярам sealed и open.
type Stock struct {
	// Total — сумма remaining_qty по sealed + open.
	Total int64 `json:"total"`
	// OpenQty и OpenCapacity — остаток и объём вскрытого экземпляра
	// (самого раннего по opened_at); null — вскрытого нет.
	OpenQty      *int64 `json:"open_qty"`
	OpenCapacity *int64 `json:"open_capacity"`
	SealedCount  int    `json:"sealed_count"`
	UnitsCount   int    `json:"units_count"`
}

// ProductSummary — строка списка остатков (docs/API.md).
type ProductSummary struct {
	ID          int64  `json:"id"`
	CategoryID  int64  `json:"category_id"`
	Name        string `json:"name"`
	Brand       string `json:"brand"`
	Unit        string `json:"unit"`
	PackageSize int64  `json:"package_size"`
	Barcode     string `json:"barcode"`
	MinStock    *int64 `json:"min_stock"`
	Note        string `json:"note"`
	Archived    bool   `json:"archived"`
	Stock       Stock  `json:"stock"`
	// Low — min_stock задан и total < min_stock.
	Low bool `json:"low"`
	// LocationIDs — места sealed/open экземпляров, без повторов, по возрастанию.
	LocationIDs []int64 `json:"location_ids"`
	// LastConsumeQty — сколько списали последним ручным расходом; null — не списывали.
	LastConsumeQty *int64 `json:"last_consume_qty"`
}

// Unit — экземпляр (бутылка или пачка).
type Unit struct {
	ID           int64  `json:"id"`
	ProductID    int64  `json:"product_id"`
	LocationID   *int64 `json:"location_id"`
	InitialQty   int64  `json:"initial_qty"`
	RemainingQty int64  `json:"remaining_qty"`
	Status       string `json:"status"`
	OpenedAt     *int64 `json:"opened_at"`
	CreatedAt    int64  `json:"created_at"`
	// UnitPrice — цена упаковки в копейках; null — экземпляр без покупки.
	UnitPrice *int64 `json:"unit_price"`
}

// Movement — запись журнала движений.
type Movement struct {
	ID             int64  `json:"id"`
	UnitID         int64  `json:"unit_id"`
	ProductID      int64  `json:"product_id"`
	Type           string `json:"type"`
	QtyDelta       int64  `json:"qty_delta"`
	FromLocationID *int64 `json:"from_location_id"`
	ToLocationID   *int64 `json:"to_location_id"`
	Reason         string `json:"reason"`
	RecipeRunID    *int64 `json:"recipe_run_id"`
	// Cost — |qty_delta| × цена упаковки ÷ initial_qty, к ближайшему; null — нет цены.
	Cost      *int64 `json:"cost"`
	CreatedAt int64  `json:"created_at"`
}

// PurchaseRow — строка истории покупок позиции.
type PurchaseRow struct {
	PurchaseID      int64   `json:"purchase_id"`
	Date            string  `json:"date"`
	Store           *string `json:"store"` // null — покупка без магазина
	Packages        int64   `json:"packages"`
	PricePerPackage int64   `json:"price_per_package"`
	// PricePer100 — для мл, PricePerUnit — для шт; второе поле не выводится.
	PricePer100  *int64 `json:"price_per_100,omitempty"`
	PricePerUnit *int64 `json:"price_per_unit,omitempty"`
}

const productColumns = `id, category_id, name, brand, unit, package_size, barcode, min_stock, note, archived`

func scanProduct(sc interface{ Scan(...any) error }) (ProductSummary, error) {
	var p ProductSummary
	var minStock sql.NullInt64
	err := sc.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Brand, &p.Unit, &p.PackageSize,
		&p.Barcode, &minStock, &p.Note, &p.Archived)
	p.MinStock = nullInt(minStock)
	p.LocationIDs = []int64{}
	return p, err
}

// Summaries возвращает сводки позиций в порядке ids. Чужие и
// несуществующие id пропускаются, повторы — тоже.
func (s *Service) Summaries(ctx context.Context, userID int64, ids []int64) ([]ProductSummary, error) {
	out := []ProductSummary{}
	if len(ids) == 0 {
		return out, nil
	}
	in := placeholders(len(ids))
	args := idArgs(userID, ids)

	rows, err := s.q.QueryContext(ctx,
		`SELECT `+productColumns+` FROM products WHERE user_id = ? AND id IN (`+in+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("сводка позиций: %w", err)
	}
	byID := map[int64]*ProductSummary{}
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("сводка позиций: %w", err)
		}
		byID[p.ID] = &p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("сводка позиций: %w", err)
	}
	if err := s.fillStock(ctx, userID, in, args, byID); err != nil {
		return nil, err
	}
	if err := s.fillLastConsume(ctx, userID, in, args, byID); err != nil {
		return nil, err
	}

	seen := map[int64]bool{}
	for _, id := range ids {
		if p, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			p.Low = p.MinStock != nil && p.Stock.Total < *p.MinStock
			out = append(out, *p)
		}
	}
	return out, nil
}

// fillStock считает остаток по экземплярам sealed/open. Вскрытый для
// open_qty — самый ранний по opened_at (без opened_at — в конце), затем по id:
// из него же списывает расход.
func (s *Service) fillStock(ctx context.Context, userID int64, in string, args []any, byID map[int64]*ProductSummary) error {
	rows, err := s.q.QueryContext(ctx, `
		SELECT product_id, location_id, initial_qty, remaining_qty, status
		FROM units
		WHERE user_id = ? AND product_id IN (`+in+`) AND status IN ('sealed', 'open')
		ORDER BY opened_at IS NULL, opened_at, id`, args...)
	if err != nil {
		return fmt.Errorf("остатки позиций: %w", err)
	}
	defer rows.Close()
	locs := map[int64]map[int64]bool{}
	for rows.Next() {
		var productID, initial, remaining int64
		var loc sql.NullInt64
		var status string
		if err := rows.Scan(&productID, &loc, &initial, &remaining, &status); err != nil {
			return fmt.Errorf("остатки позиций: %w", err)
		}
		p := byID[productID]
		if p == nil {
			continue
		}
		p.Stock.Total += remaining
		p.Stock.UnitsCount++
		if status == "sealed" {
			p.Stock.SealedCount++
		} else if p.Stock.OpenQty == nil {
			p.Stock.OpenQty = &remaining
			p.Stock.OpenCapacity = &initial
		}
		if loc.Valid {
			if locs[productID] == nil {
				locs[productID] = map[int64]bool{}
			}
			if !locs[productID][loc.Int64] {
				locs[productID][loc.Int64] = true
				p.LocationIDs = append(p.LocationIDs, loc.Int64)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("остатки позиций: %w", err)
	}
	for _, p := range byID {
		sort.Slice(p.LocationIDs, func(i, j int) bool { return p.LocationIDs[i] < p.LocationIDs[j] })
	}
	return nil
}

// fillLastConsume — последний ручной расход (ФТ-5.8): по позиции берётся
// самое позднее по id движение consume без recipe_run_id, и суммируются все
// движения его группы (batch_id — одно нажатие «Списать», которое пишет по
// движению на каждый затронутый экземпляр). Время не годится: два нажатия
// в одну секунду склеились бы.
func (s *Service) fillLastConsume(ctx context.Context, userID int64, in string, args []any, byID map[int64]*ProductSummary) error {
	full := append(append([]any{}, args...), args...)
	// В подзапросе MAX(m2.id) — «голый» batch_id SQLite берёт из той же
	// строки, что и максимум.
	rows, err := s.q.QueryContext(ctx, `
		SELECT u.product_id, SUM(-m.qty_delta)
		FROM movements m JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
		WHERE m.user_id = ? AND u.product_id IN (`+in+`)
		  AND m.type = 'consume' AND m.recipe_run_id IS NULL
		  AND (u.product_id, COALESCE(m.batch_id, m.id)) IN (
			SELECT product_id, batch FROM (
				SELECT u2.product_id, COALESCE(m2.batch_id, m2.id) AS batch, MAX(m2.id)
				FROM movements m2 JOIN units u2 ON u2.user_id = m2.user_id AND u2.id = m2.unit_id
				WHERE m2.user_id = ? AND u2.product_id IN (`+in+`)
				  AND m2.type = 'consume' AND m2.recipe_run_id IS NULL
				GROUP BY u2.product_id))
		GROUP BY u.product_id`, full...)
	if err != nil {
		return fmt.Errorf("последний расход: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var productID, qty int64
		if err := rows.Scan(&productID, &qty); err != nil {
			return fmt.Errorf("последний расход: %w", err)
		}
		if p := byID[productID]; p != nil {
			p.LastConsumeQty = &qty
		}
	}
	return rows.Err()
}

// Summary — сводка одной позиции; чужая или несуществующая — ErrNotFound.
func (s *Service) Summary(ctx context.Context, userID, productID int64) (ProductSummary, error) {
	list, err := s.Summaries(ctx, userID, []int64{productID})
	if err != nil {
		return ProductSummary{}, err
	}
	if len(list) == 0 {
		return ProductSummary{}, notFound(msgProductNotFound)
	}
	return list[0], nil
}

const unitColumns = `u.id, u.product_id, u.location_id, u.initial_qty, u.remaining_qty, u.status,
	u.opened_at, u.created_at, l.price_per_package`

const unitFrom = ` FROM units u LEFT JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id `

func scanUnit(sc interface{ Scan(...any) error }) (Unit, error) {
	var u Unit
	var loc, opened, price sql.NullInt64
	err := sc.Scan(&u.ID, &u.ProductID, &loc, &u.InitialQty, &u.RemainingQty, &u.Status,
		&opened, &u.CreatedAt, &price)
	u.LocationID, u.OpenedAt, u.UnitPrice = nullInt(loc), nullInt(opened), nullInt(price)
	return u, err
}

// UnitByID — экземпляр пользователя в любом статусе; чужой — ErrNotFound.
func (s *Service) UnitByID(ctx context.Context, userID, unitID int64) (Unit, error) {
	u, err := scanUnit(s.q.QueryRowContext(ctx,
		`SELECT `+unitColumns+unitFrom+`WHERE u.user_id = ? AND u.id = ?`, userID, unitID))
	if errors.Is(err, sql.ErrNoRows) {
		return Unit{}, notFound(msgUnitNotFound)
	}
	if err != nil {
		return Unit{}, fmt.Errorf("экземпляр: %w", err)
	}
	return u, nil
}

// productUnits — экземпляры sealed/open позиции: вскрытые первыми (по
// opened_at), затем запечатанные по дате покупки и id — в порядке расхода.
func (s *Service) productUnits(ctx context.Context, userID, productID int64) ([]Unit, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+unitColumns+unitFrom+`
		LEFT JOIN purchases p ON p.user_id = l.user_id AND p.id = l.purchase_id
		WHERE u.user_id = ? AND u.product_id = ? AND u.status IN ('sealed', 'open')
		ORDER BY u.status <> 'open', u.opened_at IS NULL, u.opened_at, p.date IS NULL, p.date, u.id`,
		userID, productID)
	if err != nil {
		return nil, fmt.Errorf("экземпляры позиции: %w", err)
	}
	defer rows.Close()
	out := []Unit{}
	for rows.Next() {
		u, err := scanUnit(rows)
		if err != nil {
			return nil, fmt.Errorf("экземпляры позиции: %w", err)
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

const movementSelect = `SELECT m.id, m.unit_id, u.product_id, m.type, m.qty_delta,
	m.from_location_id, m.to_location_id, m.reason, m.recipe_run_id, m.created_at,
	u.initial_qty, l.price_per_package
	FROM movements m
	JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
	LEFT JOIN purchase_lines l ON l.user_id = u.user_id AND l.id = u.purchase_line_id `

func (s *Service) queryMovements(ctx context.Context, query string, args ...any) ([]Movement, error) {
	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("движения: %w", err)
	}
	defer rows.Close()
	out := []Movement{}
	for rows.Next() {
		var m Movement
		var from, to, run, price sql.NullInt64
		var initial int64
		if err := rows.Scan(&m.ID, &m.UnitID, &m.ProductID, &m.Type, &m.QtyDelta, &from, &to,
			&m.Reason, &run, &m.CreatedAt, &initial, &price); err != nil {
			return nil, fmt.Errorf("движения: %w", err)
		}
		m.FromLocationID, m.ToLocationID, m.RecipeRunID = nullInt(from), nullInt(to), nullInt(run)
		if price.Valid {
			qty := m.QtyDelta
			if qty < 0 {
				qty = -qty
			}
			cost := divRound(qty, price.Int64, initial)
			m.Cost = &cost
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// MovementsByIDs — движения пользователя в порядке ids; чужие пропускаются.
func (s *Service) MovementsByIDs(ctx context.Context, userID int64, ids []int64) ([]Movement, error) {
	if len(ids) == 0 {
		return []Movement{}, nil
	}
	list, err := s.queryMovements(ctx,
		movementSelect+`WHERE m.user_id = ? AND m.id IN (`+placeholders(len(ids))+`)`, idArgs(userID, ids)...)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]Movement, len(list))
	for _, m := range list {
		byID[m.ID] = m
	}
	out := make([]Movement, 0, len(list))
	seen := map[int64]bool{}
	for _, id := range ids {
		if m, ok := byID[id]; ok && !seen[id] {
			seen[id] = true
			out = append(out, m)
		}
	}
	return out, nil
}
