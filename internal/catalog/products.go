package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
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
	// Поля v1.5 (ФТ-15), все необязательные.
	PurposeIDs   []int64
	PhX10        *int64 // pH в десятых долях, 0…140
	Incompatible string
	Instruction  string
	Rating       *int64 // 1…5
	DoNotBuy     bool
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
	// Поля v1.5: PurposeIDs nil — не менять, пустой список — снять все;
	// PhSet и RatingSet отличают «не передан» от «null».
	PurposeIDs   *[]int64
	PhSet        bool
	PhX10        *int64
	Incompatible *string
	Instruction  *string
	RatingSet    bool
	Rating       *int64
	DoNotBuy     *bool
}

// ProductFilter — параметры списка позиций (GET /api/products).
type ProductFilter struct {
	Query      string
	CategoryID int64 // 0 — любая
	LocationID int64 // 0 — любое
	Filter     string
	Archived   bool  // только архивные
	PurposeID  int64 // 0 — любое
}

// Значения ProductFilter.Filter.
const (
	FilterAll = "all"
	FilterLow = "low"
	FilterOut = "out"
	FilterIn  = "in"
)

// ProductCard — сводка и поля, которые не нужны в списке: «Не смешивать с» и
// инструкция (ФТ-15.2). Поля сводки в JSON остаются на том же уровне.
type ProductCard struct {
	ProductSummary
	Incompatible string `json:"incompatible"`
	Instruction  string `json:"instruction"`
}

// ProductDetail — карточка позиции.
type ProductDetail struct {
	Product   ProductCard   `json:"product"`
	Units     []Unit        `json:"units"`
	Purchases []PurchaseRow `json:"purchases"`
	Movements []Movement    `json:"movements"`
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
	if in.PhX10 != nil && (*in.PhX10 < 0 || *in.PhX10 > 140) {
		return invalid("ph", errPhText)
	}
	if in.Incompatible, err = optText("incompatible", in.Incompatible, 300, "«Не смешивать с»"); err != nil {
		return err
	}
	if in.Instruction, err = optText("instruction", in.Instruction, 2000, "Инструкция"); err != nil {
		return err
	}
	if in.Rating != nil && (*in.Rating < 1 || *in.Rating > 5) {
		return invalid("rating", "Оценка — от 1 до 5 или пусто")
	}
	return nil
}

const errPhText = "pH — число от 0 до 14 с одним знаком после запятой или пусто"

// PhX10 переводит pH в десятые доли: 7,3 → 73. Число вне 0…14 или с
// большим числом знаков — ошибка проверки (поле ph).
func PhX10(v float64) (int64, error) {
	x := math.Round(v * 10)
	if math.IsNaN(v) || v < 0 || v > 14 || math.Abs(v*10-x) > 1e-6 {
		return 0, invalid("ph", errPhText)
	}
	return int64(x), nil
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
	var out ProductSummary
	err := s.inTxSvc(ctx, func(t *Service) error {
		purposes, err := t.checkPurposeIDs(ctx, userID, 0, in.PurposeIDs)
		if err != nil {
			return err
		}
		res, err := t.q.ExecContext(ctx, `INSERT INTO products
			(user_id, category_id, name, brand, unit, package_size, barcode, min_stock, note, created_at,
			 ph_x10, incompatible, instruction, rating, do_not_buy)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			userID, in.CategoryID, in.Name, in.Brand, in.Unit, in.PackageSize, in.Barcode, in.MinStock, in.Note, at,
			in.PhX10, in.Incompatible, in.Instruction, in.Rating, in.DoNotBuy)
		if err != nil {
			return fmt.Errorf("создание позиции: %w", err)
		}
		id, _ := res.LastInsertId()
		if err := t.setProductPurposes(ctx, userID, id, purposes); err != nil {
			return err
		}
		out, err = t.Summary(ctx, userID, id)
		return err
	})
	return out, err
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
	card, err := s.Card(ctx, userID, id)
	if err != nil {
		return ProductSummary{}, err
	}
	in := ProductInput{CategoryID: cur.CategoryID, Name: cur.Name, Brand: cur.Brand, Unit: cur.Unit,
		PackageSize: cur.PackageSize, Barcode: cur.Barcode, MinStock: cur.MinStock, Note: cur.Note,
		Incompatible: card.Incompatible, Instruction: card.Instruction, Rating: cur.Rating, DoNotBuy: cur.DoNotBuy}
	if cur.PH != nil {
		v := int64(math.Round(*cur.PH * 10))
		in.PhX10 = &v
	}
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
	set(&in.Incompatible, p.Incompatible)
	set(&in.Instruction, p.Instruction)
	if p.PhSet {
		in.PhX10 = p.PhX10
	}
	if p.RatingSet {
		in.Rating = p.Rating
	}
	if p.DoNotBuy != nil {
		in.DoNotBuy = *p.DoNotBuy
	}
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
	var purposes []int64
	if p.PurposeIDs != nil {
		if purposes, err = s.checkPurposeIDs(ctx, userID, id, *p.PurposeIDs); err != nil {
			return ProductSummary{}, err
		}
	}
	if _, err := s.q.ExecContext(ctx, `UPDATE products SET category_id = ?, name = ?, brand = ?, unit = ?,
		package_size = ?, barcode = ?, min_stock = ?, note = ?, archived = ?,
		ph_x10 = ?, incompatible = ?, instruction = ?, rating = ?, do_not_buy = ?
		WHERE user_id = ? AND id = ?`,
		in.CategoryID, in.Name, in.Brand, in.Unit, in.PackageSize, in.Barcode, in.MinStock, in.Note, archived,
		in.PhX10, in.Incompatible, in.Instruction, in.Rating, in.DoNotBuy,
		userID, id); err != nil {
		return ProductSummary{}, fmt.Errorf("изменение позиции: %w", err)
	}
	if p.PurposeIDs != nil {
		if err := s.setProductPurposes(ctx, userID, id, purposes); err != nil {
			return ProductSummary{}, err
		}
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
	var purposeNames map[int64]string
	if q != "" {
		if purposeNames, err = s.purposeNames(ctx, userID); err != nil {
			return nil, err
		}
	}
	out := []ProductSummary{}
	for _, p := range all {
		if q != "" && !strings.Contains(strings.ToLower(p.Name), q) &&
			!strings.Contains(strings.ToLower(p.Brand), q) && !strings.Contains(p.Barcode, q) &&
			!purposeMatches(p.PurposeIDs, purposeNames, q) {
			continue
		}
		if f.PurposeID != 0 && !containsID(p.PurposeIDs, f.PurposeID) {
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

// purposeMatches — в имени какого-нибудь назначения позиции есть q (нижний регистр).
func purposeMatches(ids []int64, names map[int64]string, q string) bool {
	for _, id := range ids {
		if strings.Contains(strings.ToLower(names[id]), q) {
			return true
		}
	}
	return false
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
	card, err := s.Card(ctx, userID, id)
	if err != nil {
		return ProductDetail{}, err
	}
	sum := card.ProductSummary
	d := ProductDetail{Product: card}
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

// Brands — бренды неархивных позиций пользователя без повторов (без учёта
// регистра), по алфавиту. Из вариантов написания одного бренда берётся
// самый частый, при равенстве — первый по алфавиту.
func (s *Service) Brands(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.q.QueryContext(ctx, `
		SELECT brand FROM products WHERE user_id = ? AND brand <> '' AND archived = 0
		GROUP BY brand ORDER BY COUNT(*) DESC, brand`, userID)
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

// Card — сводка позиции с «Не смешивать с» и инструкцией; чужая — ErrNotFound.
func (s *Service) Card(ctx context.Context, userID, id int64) (ProductCard, error) {
	sum, err := s.Summary(ctx, userID, id)
	if err != nil {
		return ProductCard{}, err
	}
	c := ProductCard{ProductSummary: sum}
	if err := s.q.QueryRowContext(ctx, `SELECT incompatible, instruction FROM products WHERE user_id = ? AND id = ?`,
		userID, id).Scan(&c.Incompatible, &c.Instruction); err != nil {
		return ProductCard{}, fmt.Errorf("карточка позиции: %w", err)
	}
	return c, nil
}

// SetRating меняет оценку и/или «Не брать больше» (ФТ-15.4): звёзды прямо
// в карточке. ratingSet — оценка передана (nil — снять); doNotBuy nil — не менять.
func (s *Service) SetRating(ctx context.Context, userID, id int64, ratingSet bool, rating *int64, doNotBuy *bool) (ProductSummary, error) {
	return s.UpdateProduct(ctx, userID, id, ProductPatch{RatingSet: ratingSet, Rating: rating, DoNotBuy: doNotBuy})
}
