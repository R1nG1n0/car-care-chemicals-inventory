package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ProductInput — поля позиции для создания; для изменения — ProductPatch.
type ProductInput struct {
	CategoryID  int64
	Name        string
	Brand       string
	Unit        string
	PackageSize int64
	Barcode     string
	MinStock    *int64
	Note        string
}

// ProductPatch — изменение позиции; nil — поле не меняется. MinStockSet
// отличает «не передан» от «null» (сбросить минимум).
type ProductPatch struct {
	CategoryID  *int64
	Name        *string
	Brand       *string
	Unit        *string
	PackageSize *int64
	Barcode     *string
	MinStockSet bool
	MinStock    *int64
	Note        *string
	Archived    *bool
}

// ProductFilter — параметры списка позиций (GET /api/products).
type ProductFilter struct {
	Query      string
	CategoryID int64 // 0 — любая
	LocationID int64 // 0 — любое
	Filter     string
	Archived   bool // только архивные
}

// Значения ProductFilter.Filter.
const (
	FilterAll = "all"
	FilterLow = "low"
	FilterOut = "out"
	FilterIn  = "in"
)

// ProductDetail — карточка позиции.
type ProductDetail struct {
	Product   ProductSummary `json:"product"`
	Units     []Unit         `json:"units"`
	Purchases []PurchaseRow  `json:"purchases"`
	Movements []Movement     `json:"movements"`
}

const (
	detailPurchases = 20
	detailMovements = 50
)

func (in *ProductInput) validate() error {
	var err error
	if in.Name, err = cleanName("name", in.Name, 80, "Название"); err != nil {
		return err
	}
	if in.Brand, err = optText("brand", in.Brand, 60, "Бренд"); err != nil {
		return err
	}
	if in.Unit != "ml" && in.Unit != "pcs" {
		return invalid("unit", "Единица — мл или шт")
	}
	if in.PackageSize < 1 || in.PackageSize > 100000 {
		return invalid("package_size", "Размер упаковки — целое число от 1 до 100000")
	}
	in.Barcode = strings.TrimSpace(in.Barcode)
	if len(in.Barcode) > 32 || strings.Trim(in.Barcode, "0123456789") != "" {
		return invalid("barcode", "Штрихкод — до 32 цифр")
	}
	if in.MinStock != nil && (*in.MinStock < 0 || *in.MinStock > 1000000) {
		return invalid("min_stock", "Минимальный остаток — пусто или целое число от 0 до 1000000")
	}
	if in.Note, err = optText("note", in.Note, 500, "Заметка"); err != nil {
		return err
	}
	return nil
}

// checkCategory: категория своя (иначе 404) и действующая (иначе 400).
func (s *Service) checkCategory(ctx context.Context, userID, id int64) error {
	var archived bool
	err := s.q.QueryRowContext(ctx, `SELECT archived FROM categories WHERE user_id = ? AND id = ?`, userID, id).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound(msgCategoryNotFound)
	}
	if err != nil {
		return fmt.Errorf("категория позиции: %w", err)
	}
	if archived {
		return invalid("category_id", "Категория в архиве")
	}
	return nil
}

// CreateProduct добавляет позицию.
func (s *Service) CreateProduct(ctx context.Context, userID int64, in ProductInput, at int64) (ProductSummary, error) {
	if err := in.validate(); err != nil {
		return ProductSummary{}, err
	}
	if err := s.checkCategory(ctx, userID, in.CategoryID); err != nil {
		return ProductSummary{}, err
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO products
		(user_id, category_id, name, brand, unit, package_size, barcode, min_stock, note, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, in.CategoryID, in.Name, in.Brand, in.Unit, in.PackageSize, in.Barcode, in.MinStock, in.Note, at)
	if err != nil {
		return ProductSummary{}, fmt.Errorf("создание позиции: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Summary(ctx, userID, id)
}

// UpdateProduct меняет поля позиции. Единицу нельзя менять, когда у позиции
// есть экземпляры (409); новый размер упаковки экземпляры не трогает.
// Проверка и запись — одна транзакция: покупка позиции не вклинится между ними.
func (s *Service) UpdateProduct(ctx context.Context, userID, id int64, p ProductPatch) (ProductSummary, error) {
	var out ProductSummary
	err := s.inTxSvc(ctx, func(t *Service) (err error) {
		out, err = t.updateProduct(ctx, userID, id, p)
		return err
	})
	return out, err
}

func (s *Service) updateProduct(ctx context.Context, userID, id int64, p ProductPatch) (ProductSummary, error) {
	cur, err := s.Summary(ctx, userID, id)
	if err != nil {
		return ProductSummary{}, err
	}
	in := ProductInput{CategoryID: cur.CategoryID, Name: cur.Name, Brand: cur.Brand, Unit: cur.Unit,
		PackageSize: cur.PackageSize, Barcode: cur.Barcode, MinStock: cur.MinStock, Note: cur.Note}
	set := func(dst *string, v *string) {
		if v != nil {
			*dst = *v
		}
	}
	set(&in.Name, p.Name)
	set(&in.Brand, p.Brand)
	set(&in.Unit, p.Unit)
	set(&in.Barcode, p.Barcode)
	set(&in.Note, p.Note)
	if p.PackageSize != nil {
		in.PackageSize = *p.PackageSize
	}
	if p.MinStockSet {
		in.MinStock = p.MinStock
	}
	if err := in.validate(); err != nil {
		return ProductSummary{}, err
	}
	if p.CategoryID != nil && *p.CategoryID != cur.CategoryID {
		if err := s.checkCategory(ctx, userID, *p.CategoryID); err != nil {
			return ProductSummary{}, err
		}
		in.CategoryID = *p.CategoryID
	}
	if in.Unit != cur.Unit {
		var n int
		if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM units WHERE user_id = ? AND product_id = ?`,
			userID, id).Scan(&n); err != nil {
			return ProductSummary{}, fmt.Errorf("изменение позиции: %w", err)
		}
		if n > 0 {
			return ProductSummary{}, conflict("unit", "Нельзя сменить единицу: у позиции есть экземпляры")
		}
	}
	archived := cur.Archived
	if p.Archived != nil {
		archived = *p.Archived
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE products SET category_id = ?, name = ?, brand = ?, unit = ?,
		package_size = ?, barcode = ?, min_stock = ?, note = ?, archived = ? WHERE user_id = ? AND id = ?`,
		in.CategoryID, in.Name, in.Brand, in.Unit, in.PackageSize, in.Barcode, in.MinStock, in.Note, archived,
		userID, id); err != nil {
		return ProductSummary{}, fmt.Errorf("изменение позиции: %w", err)
	}
	return s.Summary(ctx, userID, id)
}

// Products — список сводок по имени. Поиск и фильтры по остатку — в Go:
// LOWER в SQLite знает только латиницу, а позиций у пользователя десятки.
func (s *Service) Products(ctx context.Context, userID int64, f ProductFilter) ([]ProductSummary, error) {
	switch f.Filter {
	case "":
		f.Filter = FilterAll
	case FilterAll, FilterLow, FilterOut, FilterIn:
	default:
		return nil, invalid("filter", "Фильтр — all, low, out или in")
	}
	rows, err := s.q.QueryContext(ctx, `SELECT id FROM products
		WHERE user_id = ? AND archived = ? AND (? = 0 OR category_id = ?)`,
		userID, f.Archived, f.CategoryID, f.CategoryID)
	if err != nil {
		return nil, fmt.Errorf("список позиций: %w", err)
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("список позиций: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("список позиций: %w", err)
	}
	all, err := s.Summaries(ctx, userID, ids)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(f.Query))
	out := []ProductSummary{}
	for _, p := range all {
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) &&
			!strings.Contains(strings.ToLower(p.Brand), q) && !strings.Contains(p.Barcode, q) {
			continue
		}
		if f.LocationID != 0 && !containsID(p.LocationIDs, f.LocationID) {
			continue
		}
		switch {
		case f.Filter == FilterLow && !p.Low,
			f.Filter == FilterOut && p.Stock.Total != 0,
			f.Filter == FilterIn && p.Stock.Total == 0:
			continue
		}
		out = append(out, p)
	}
	sortByName(out)
	return out, nil
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// sortByName — по имени без учёта регистра, затем по id.
func sortByName(list []ProductSummary) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)
		if a != b {
			return a < b
		}
		return list[i].ID < list[j].ID
	})
}

// Product — карточка позиции: сводка, экземпляры, покупки, движения.
func (s *Service) Product(ctx context.Context, userID, id int64) (ProductDetail, error) {
	sum, err := s.Summary(ctx, userID, id)
	if err != nil {
		return ProductDetail{}, err
	}
	d := ProductDetail{Product: sum}
	if d.Units, err = s.productUnits(ctx, userID, id); err != nil {
		return ProductDetail{}, err
	}
	if d.Purchases, err = s.productPurchases(ctx, userID, sum); err != nil {
		return ProductDetail{}, err
	}
	if d.Movements, err = s.queryMovements(ctx, movementSelect+
		`WHERE m.user_id = ? AND u.product_id = ? ORDER BY m.created_at DESC, m.id DESC LIMIT ?`,
		userID, id, detailMovements); err != nil {
		return ProductDetail{}, err
	}
	return d, nil
}

// lineSizeExpr — объём упаковки строки покупки: initial_qty её экземпляров,
// а если их нет — текущий package_size позиции (он мог поменяться позже).
const lineSizeExpr = `COALESCE((SELECT MAX(u.initial_qty) FROM units u
	WHERE u.user_id = l.user_id AND u.purchase_line_id = l.id), pr.package_size)`

func (s *Service) productPurchases(ctx context.Context, userID int64, p ProductSummary) ([]PurchaseRow, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT pu.id, pu.date, st.name, l.packages, l.price_per_package, `+lineSizeExpr+`
		FROM purchase_lines l
		JOIN purchases pu ON pu.user_id = l.user_id AND pu.id = l.purchase_id
		JOIN products pr ON pr.user_id = l.user_id AND pr.id = l.product_id
		LEFT JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE l.user_id = ? AND l.product_id = ?
		ORDER BY pu.date DESC, pu.id DESC, l.id DESC LIMIT ?`, userID, p.ID, detailPurchases)
	if err != nil {
		return nil, fmt.Errorf("покупки позиции: %w", err)
	}
	defer rows.Close()
	out := []PurchaseRow{}
	for rows.Next() {
		var r PurchaseRow
		var store sql.NullString
		var size int64
		if err := rows.Scan(&r.PurchaseID, &r.Date, &store, &r.Packages, &r.PricePerPackage, &size); err != nil {
			return nil, fmt.Errorf("покупки позиции: %w", err)
		}
		r.Store = nullString(store)
		if p.Unit == "ml" {
			v := divRound(r.PricePerPackage, 100, size)
			r.PricePer100 = &v
		} else {
			v := divRound(r.PricePerPackage, 1, size)
			r.PricePerUnit = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Brands — бренды позиций пользователя без повторов (без учёта регистра),
// по алфавиту.
func (s *Service) Brands(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT DISTINCT brand FROM products WHERE user_id = ? AND brand <> ''`, userID)
	if err != nil {
		return nil, fmt.Errorf("бренды: %w", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	out := []string{}
	for rows.Next() {
		var b string
		if err := rows.Scan(&b); err != nil {
			return nil, fmt.Errorf("бренды: %w", err)
		}
		if k := strings.ToLower(b); !seen[k] {
			seen[k] = true
			out = append(out, b)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("бренды: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out, nil
}
