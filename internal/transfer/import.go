package transfer

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// NotEmptyError — склад не пуст, импорт невозможен (409).
type NotEmptyError struct{ Message string }

func (e *NotEmptyError) Error() string { return e.Message }

// Counts — сколько записей каждого вида в файле или импортировано.
type Counts struct {
	Categories    int `json:"categories"`
	Locations     int `json:"locations"`
	Stores        int `json:"stores"`
	Purposes      int `json:"purposes"`
	Products      int `json:"products"`
	Units         int `json:"units"`
	Purchases     int `json:"purchases"`
	PurchaseLines int `json:"purchase_lines"`
	Movements     int `json:"movements"`
	Recipes       int `json:"recipes"`
	Shopping      int `json:"shopping"`
}

// Count считает записи файла.
func Count(f *File) Counts {
	c := Counts{
		Categories: len(f.Categories), Locations: len(f.Locations), Stores: len(f.Stores),
		Purposes: len(f.Purposes), Products: len(f.Products), Units: len(f.Units), Purchases: len(f.Purchases),
		Movements: len(f.Movements), Recipes: len(f.Recipes), Shopping: len(f.Shopping),
	}
	for _, p := range f.Purchases {
		c.PurchaseLines += len(p.Lines)
	}
	return c
}

// importer ведёт карты «id в файле → id в базе» в одной транзакции.
type importer struct {
	ctx   context.Context
	tx    *sql.Tx
	uid   int64
	now   int64
	stmts []*sql.Stmt

	categories, locations, stores, purposes, products, lines, units, runs, movements, recipes map[int64]int64
}

// BeginImport занимает единственное место для импорта: большой файл держит
// память и единственное соединение с базой, поэтому на сервере идёт не больше
// одного импорта. release отпускает место; ok == false — место занято.
func (s *Service) BeginImport() (release func(), ok bool) {
	if !s.importing.CompareAndSwap(false, true) {
		return nil, false
	}
	return func() { s.importing.Store(false) }, true
}

// CheckEmpty — дешёвая проверка «склад пуст» без транзакции: чтобы отказать
// до чтения и разбора большого файла. Решающая проверка — внутри Import.
func (s *Service) CheckEmpty(ctx context.Context, userID int64) error {
	return checkEmpty(ctx, s.db, userID)
}

// Import проверяет файл и переносит его в склад пользователя одной
// транзакцией. Склад должен быть пустым: ни одной позиции, покупки,
// движения. Категории, места и магазины
// с совпадающим именем (без учёта регистра) сливаются: стандартные категории,
// созданные при регистрации, не мешают. Ошибка — ничего не записано.
//
// Два импорта в одного пользователя не создадут дубль только потому, что у
// базы одно соединение (store.Open: MaxOpenConns(1)): вторая транзакция
// начинается после завершения первой, и checkEmpty внутри неё уже видит
// данные. Если число соединений когда-либо вырастет, нужна блокировка записи
// (BEGIN IMMEDIATE), иначе эта гарантия пропадёт; тест
// TestImportParallelOneWinner это проверяет.
func (s *Service) Import(ctx context.Context, userID int64, f *File) (Counts, error) {
	if err := Validate(f); err != nil {
		return Counts{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Counts{}, fmt.Errorf("импорт: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // после Commit — no-op
	if err := checkEmpty(ctx, tx, userID); err != nil {
		return Counts{}, err
	}
	im := &importer{ctx: ctx, tx: tx, uid: userID, now: s.now().UnixMilli(),
		categories: map[int64]int64{}, locations: map[int64]int64{}, stores: map[int64]int64{},
		purposes: map[int64]int64{}, products: map[int64]int64{}, lines: map[int64]int64{}, units: map[int64]int64{},
		runs: map[int64]int64{}, movements: map[int64]int64{}, recipes: map[int64]int64{},
	}
	defer im.closeStmts()
	for _, step := range []func(*File) error{
		im.settings, im.refs, im.productsStep, im.purchases, im.unitsStep, im.recipesStep, im.movementsStep,
		im.shopping,
	} {
		if err := step(f); err != nil {
			return Counts{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Counts{}, fmt.Errorf("импорт: фиксация: %w", err)
	}
	return Count(f), nil
}

// checkEmpty: склад пуст, если у пользователя нет ни позиций (в том числе
// архивных), ни покупок, ни движений. Справочники (категории, места, магазины)
// не считаются: они сливаются по имени. Шаблоны расхода и пункты списка
// покупок тоже не считаются: шаблон с совпадающим названием отклоняется
// ошибкой 400 уже при записи (откат полный), остальное дописывается.
func checkEmpty(ctx context.Context, q querier, uid int64) error {
	var products, purchases, movements int
	err := q.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM products WHERE user_id = ?),
		(SELECT COUNT(*) FROM purchases WHERE user_id = ?),
		(SELECT COUNT(*) FROM movements WHERE user_id = ?)`, uid, uid, uid).Scan(&products, &purchases, &movements)
	if err != nil {
		return fmt.Errorf("импорт: проверка склада: %w", err)
	}
	if products+purchases+movements > 0 {
		return &NotEmptyError{Message: fmt.Sprintf("Импорт возможен только в пустой склад. У вас уже есть данные: "+
			"позиций — %d, покупок — %d, движений — %d. Войдите под новым пользователем и загрузите файл там.",
			products, purchases, movements)}
	}
	return nil
}

// prepare готовит запрос для цикла вставки; закрывается в конце импорта.
func (im *importer) prepare(what, query string) (*sql.Stmt, error) {
	st, err := im.tx.PrepareContext(im.ctx, query)
	if err != nil {
		return nil, fmt.Errorf("импорт, %s: %w", what, err)
	}
	im.stmts = append(im.stmts, st)
	return st, nil
}

func (im *importer) closeStmts() {
	for _, st := range im.stmts {
		_ = st.Close()
	}
}

// run выполняет подготовленный запрос и возвращает id вставленной строки.
func (im *importer) run(st *sql.Stmt, what string, args ...any) (int64, error) {
	res, err := st.ExecContext(im.ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("импорт, %s: %w", what, err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (im *importer) exec(what, query string, args ...any) (int64, error) {
	res, err := im.tx.ExecContext(im.ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("импорт, %s: %w", what, err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

func (im *importer) settings(f *File) error {
	if f.Settings == nil {
		return nil
	}
	_, err := im.exec("настройки", `INSERT INTO user_settings (user_id, step_small, step_large) VALUES (?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET step_small = excluded.step_small, step_large = excluded.step_large`,
		im.uid, f.Settings.StepSmall, f.Settings.StepLarge)
	return err
}

// existing — имена справочника пользователя (в нижнем регистре) → id.
func (im *importer) existing(table string) (map[string]int64, error) {
	rows, err := im.tx.QueryContext(im.ctx, `SELECT id, name FROM `+table+` WHERE user_id = ?`, im.uid)
	if err != nil {
		return nil, fmt.Errorf("импорт, %s: %w", table, err)
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("импорт, %s: %w", table, err)
		}
		out[strings.ToLower(name)] = id
	}
	return out, rows.Err()
}

func (im *importer) refs(f *File) error {
	cats, err := im.existing("categories")
	if err != nil {
		return err
	}
	for _, c := range f.Categories {
		if id, ok := cats[strings.ToLower(c.Name)]; ok {
			im.categories[c.ID] = id
			_, err = im.exec("категория", `UPDATE categories SET color = ?, sort = ?, archived = ? WHERE user_id = ? AND id = ?`,
				c.Color, c.Sort, c.Archived, im.uid, id)
		} else {
			im.categories[c.ID], err = im.exec("категория", `INSERT INTO categories (user_id, name, color, sort, archived)
				VALUES (?, ?, ?, ?, ?)`, im.uid, c.Name, c.Color, c.Sort, c.Archived)
		}
		if err != nil {
			return err
		}
	}
	locs, err := im.existing("locations")
	if err != nil {
		return err
	}
	for _, l := range f.Locations {
		if id, ok := locs[strings.ToLower(l.Name)]; ok {
			im.locations[l.ID] = id
			_, err = im.exec("место", `UPDATE locations SET sort = ?, archived = ?, checked_at = ? WHERE user_id = ? AND id = ?`,
				l.Sort, l.Archived, l.CheckedAt, im.uid, id)
		} else {
			im.locations[l.ID], err = im.exec("место", `INSERT INTO locations (user_id, name, sort, archived, checked_at)
				VALUES (?, ?, ?, ?, ?)`, im.uid, l.Name, l.Sort, l.Archived, l.CheckedAt)
		}
		if err != nil {
			return err
		}
	}
	stores, err := im.existing("stores")
	if err != nil {
		return err
	}
	for _, s := range f.Stores {
		if id, ok := stores[strings.ToLower(s.Name)]; ok {
			im.stores[s.ID] = id
			_, err = im.exec("магазин", `UPDATE stores SET url = ?, archived = ? WHERE user_id = ? AND id = ?`,
				s.URL, s.Archived, im.uid, id)
		} else {
			im.stores[s.ID], err = im.exec("магазин", `INSERT INTO stores (user_id, name, url, archived)
				VALUES (?, ?, ?, ?)`, im.uid, s.Name, s.URL, s.Archived)
		}
		if err != nil {
			return err
		}
	}
	return im.purposesRefs(f)
}

// purposesRefs сливает назначения файла с имеющимися по имени, как категории:
// стартовый набор из миграции не мешает, а получает порядок и архив из файла.
func (im *importer) purposesRefs(f *File) error {
	existing, err := im.existing("purposes")
	if err != nil {
		return err
	}
	for _, p := range f.Purposes {
		if id, ok := existing[strings.ToLower(p.Name)]; ok {
			im.purposes[p.ID] = id
			_, err = im.exec("назначение", `UPDATE purposes SET sort = ?, archived = ? WHERE user_id = ? AND id = ?`,
				p.Sort, p.Archived, im.uid, id)
		} else {
			im.purposes[p.ID], err = im.exec("назначение", `INSERT INTO purposes (user_id, name, sort, archived)
				VALUES (?, ?, ?, ?)`, im.uid, p.Name, p.Sort, p.Archived)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// mapped — id в базе для необязательной ссылки; nil остаётся NULL.
func mapped(m map[int64]int64, id *int64) any {
	if id == nil {
		return nil
	}
	return m[*id]
}

func (im *importer) productsStep(f *File) error {
	ins, err := im.prepare("позиция", `INSERT INTO products
		(user_id, category_id, name, brand, unit, package_size, barcode, min_stock, note, archived, created_at,
		 ph_x10, incompatible, instruction, rating, do_not_buy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	link, err := im.prepare("назначение позиции", `INSERT INTO product_purposes (user_id, product_id, purpose_id)
		VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	dil, err := im.prepare("пропорция позиции", `INSERT INTO product_dilutions (user_id, product_id, name, ratio, sort)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	ph, err := im.prepare("фото позиции", `INSERT INTO product_photos
		(user_id, product_id, mime, data, width, height, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, p := range f.Products {
		var phX10 *int64 // pH уже проверен Validate
		if p.PH != nil {
			x, _ := catalog.PhX10(*p.PH)
			phX10 = &x
		}
		id, err := im.run(ins, "позиция", im.uid, im.categories[p.CategoryID], p.Name, p.Brand, p.Unit, p.PackageSize,
			p.Barcode, p.MinStock, p.Note, p.Archived, p.CreatedAt, phX10, p.Incompatible, p.Instruction, p.Rating,
			p.DoNotBuy)
		if err != nil {
			return err
		}
		im.products[p.ID] = id
		for _, pid := range p.PurposeIDs {
			if _, err := im.run(link, "назначение позиции", im.uid, id, im.purposes[pid]); err != nil {
				return err
			}
		}
		for j, d := range p.Dilutions {
			if _, err := im.run(dil, "пропорция позиции", im.uid, id, d.Name, d.Ratio, j); err != nil {
				return err
			}
		}
		if p.Photo != nil {
			if _, err := im.run(ph, "фото позиции", im.uid, id, p.Photo.info.Mime, p.Photo.raw,
				p.Photo.info.Width, p.Photo.info.Height, im.now); err != nil {
				return err
			}
		}
	}
	return nil
}

func (im *importer) purchases(f *File) error {
	insP, err := im.prepare("покупка", `INSERT INTO purchases (user_id, store_id, date, note, created_at)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	insL, err := im.prepare("строка покупки", `INSERT INTO purchase_lines
		(user_id, purchase_id, product_id, packages, price_per_package) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, p := range f.Purchases {
		id, err := im.run(insP, "покупка", im.uid, mapped(im.stores, p.StoreID), p.Date, p.Note, p.CreatedAt)
		if err != nil {
			return err
		}
		for _, l := range p.Lines {
			im.lines[l.ID], err = im.run(insL, "строка покупки", im.uid, id, im.products[l.ProductID], l.Packages,
				l.PricePerPackage)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (im *importer) unitsStep(f *File) error {
	ins, err := im.prepare("экземпляр", `INSERT INTO units
		(user_id, product_id, purchase_line_id, location_id, initial_qty, remaining_qty, status,
		 opened_at, finished_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, u := range f.Units {
		id, err := im.run(ins, "экземпляр", im.uid, im.products[u.ProductID], mapped(im.lines, u.PurchaseLineID),
			mapped(im.locations, u.LocationID), u.InitialQty, u.RemainingQty, u.Status, u.OpenedAt, u.FinishedAt,
			u.CreatedAt)
		if err != nil {
			return err
		}
		im.units[u.ID] = id
	}
	return nil
}

func (im *importer) recipesStep(f *File) error {
	names, err := im.existing("recipes")
	if err != nil {
		return err
	}
	insItem, err := im.prepare("строка шаблона", `INSERT INTO recipe_items
		(user_id, recipe_id, product_id, qty, sort, solution_ml, solution_ratio) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for i, r := range f.Recipes {
		if _, taken := names[strings.ToLower(r.Name)]; taken {
			return bad(at("recipes", i, "name"), "Шаблон с таким названием уже есть")
		}
		id, err := im.exec("шаблон", `INSERT INTO recipes (user_id, name, sort, created_at) VALUES (?, ?, ?, ?)`,
			im.uid, r.Name, r.Sort, r.CreatedAt)
		if err != nil {
			return err
		}
		im.recipes[r.ID] = id
		for _, it := range r.Items {
			if _, err := im.run(insItem, "строка шаблона", im.uid, id, im.products[it.ProductID], it.Qty, it.Sort,
				it.SolutionML, it.SolutionRatio); err != nil {
				return err
			}
		}
	}
	for _, run := range f.RecipeRuns {
		// Шаблон мог быть удалён раньше: тогда ссылка пуста, как в исходной базе.
		var recipe any
		if run.RecipeID != nil {
			if id, ok := im.recipes[*run.RecipeID]; ok {
				recipe = id
			}
		}
		id, err := im.exec("применение шаблона", `INSERT INTO recipe_runs (user_id, recipe_id, created_at)
			VALUES (?, ?, ?)`, im.uid, recipe, run.CreatedAt)
		if err != nil {
			return err
		}
		im.runs[run.ID] = id
	}
	return nil
}

func (im *importer) movementsStep(f *File) error {
	ins, err := im.prepare("движение", `INSERT INTO movements
		(user_id, unit_id, type, qty_delta, from_location_id, to_location_id, reason, recipe_run_id, created_at,
		 solution_ml, solution_ratio)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, m := range f.Movements {
		id, err := im.run(ins, "движение", im.uid, im.units[m.UnitID], m.Type, m.QtyDelta,
			mapped(im.locations, m.FromLocationID), mapped(im.locations, m.ToLocationID), m.Reason,
			mapped(im.runs, m.RecipeRunID), m.CreatedAt, m.SolutionML, m.SolutionRatio)
		if err != nil {
			return err
		}
		im.movements[m.ID] = id
	}
	// Группа движений запроса — id первого движения: переносим через карту.
	upd, err := im.prepare("группа движений", `UPDATE movements SET batch_id = ? WHERE user_id = ? AND id = ?`)
	if err != nil {
		return err
	}
	for _, m := range f.Movements {
		batch := im.movements[m.ID]
		if m.BatchID != nil {
			batch = im.movements[*m.BatchID]
		}
		if _, err := im.run(upd, "группа движений", batch, im.uid, im.movements[m.ID]); err != nil {
			return err
		}
	}
	return nil
}

func (im *importer) shopping(f *File) error {
	ins, err := im.prepare("список покупок", `INSERT INTO shopping_items (user_id, product_id, title, done_at, created_at)
		VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	for _, it := range f.Shopping {
		if _, err := im.run(ins, "список покупок", im.uid, mapped(im.products, it.ProductID), it.Title, it.DoneAt,
			it.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}
