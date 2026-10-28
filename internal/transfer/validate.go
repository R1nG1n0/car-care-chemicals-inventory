package transfer

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/auth"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/photo"
)

// Пределы те же, что у обычного API (docs/API.md, ТБ-7).
const (
	maxQty        = 100000
	maxPrice      = 100000000
	maxPackages   = 100
	maxLines      = 30
	maxNote       = 500
	maxReason     = 80
	maxRecipeName = 60
	// maxTime — 2100-01-01: правдоподобная верхняя граница для unix-секунд.
	maxTime = 4102444800
)

// ImportError — файл не прошёл проверку; Path указывает место («products[3].name»).
type ImportError struct {
	Path    string
	Message string
}

func (e *ImportError) Error() string { return e.Path + ": " + e.Message }

func bad(path, msg string) error { return &ImportError{Path: path, Message: msg} }

func at(list string, i int, field string) string {
	if field == "" {
		return fmt.Sprintf("%s[%d]", list, i)
	}
	return fmt.Sprintf("%s[%d].%s", list, i, field)
}

// fromField переводит ошибку проверки пакетов catalog и auth в место файла.
func fromField(path string, err error) error {
	var ce *catalog.Error
	if errors.As(err, &ce) {
		if ce.Field != "" {
			path += "." + ce.Field
		}
		return bad(path, ce.Message)
	}
	var ae *auth.ValidationError
	if errors.As(err, &ae) {
		return bad(path+"."+ae.Field, ae.Message)
	}
	return err
}

func text(path, s string, min, max int, msg string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if !utf8.ValidString(s) || n < min || n > max {
		return "", bad(path, msg)
	}
	return s, nil
}

func inRange(path string, v, lo, hi int64, what string) error {
	if v < lo || v > hi {
		return bad(path, fmt.Sprintf("%s — целое число от %d до %d", what, lo, hi))
	}
	return nil
}

func checkTime(path string, v int64) error {
	return inRange(path, v, 0, maxTime, "Время")
}

func checkOptTime(path string, v *int64) error {
	if v == nil {
		return nil
	}
	return checkTime(path, *v)
}

// ids — множество id одного вида записей файла.
type ids map[int64]bool

func (s ids) add(path string, id int64) error {
	if id < 1 {
		return bad(path, "id — положительное число")
	}
	if s[id] {
		return bad(path, "id повторяется")
	}
	s[id] = true
	return nil
}

func (s ids) has(path string, id int64, what string) error {
	if !s[id] {
		return bad(path, fmt.Sprintf("%s с id %d нет в файле", what, id))
	}
	return nil
}

func (s ids) hasOpt(path string, id *int64, what string) error {
	if id == nil {
		return nil
	}
	return s.has(path, *id, what)
}

// sets — id всех видов записей, на которые ссылаются другие записи.
type sets struct {
	categories, locations, stores, purposes, products, lines, units, runs, movements ids
	lineProduct                                                                      map[int64]int64
}

// Validate проверяет файл целиком и приводит тексты к нормальному виду
// (обрезает пробелы). Ничего не пишет в базу.
func Validate(f *File) error {
	if f.Format != FormatName {
		return bad("format", "Это не файл экспорта garage: ожидается \""+FormatName+"\"")
	}
	if f.Version < MinFormatVersion || f.Version > FormatVersion {
		return bad("version", fmt.Sprintf("Версия файла %d не поддерживается, нужна %d или %d",
			f.Version, MinFormatVersion, FormatVersion))
	}
	if err := checkLimits(f); err != nil {
		return err
	}
	if f.Settings != nil {
		err := auth.ValidateSettings(auth.Settings{StepSmall: int(f.Settings.StepSmall), StepLarge: int(f.Settings.StepLarge)})
		if err != nil {
			return fromField("settings", err)
		}
	}
	st := &sets{
		categories: ids{}, locations: ids{}, stores: ids{}, purposes: ids{}, products: ids{}, lines: ids{}, units: ids{},
		runs: ids{}, movements: ids{}, lineProduct: map[int64]int64{},
	}
	for _, step := range []func(*File, *sets) error{
		validateRefs, validateProducts, validatePurchases, validateUnits, validateRecipes, validateMovements,
		validateBalances, validateShopping,
	} {
		if err := step(f, st); err != nil {
			return err
		}
	}
	return nil
}

// uniqueNames следит, чтобы имена в файле не повторялись без учёта регистра.
type uniqueNames map[string]bool

func (u uniqueNames) add(path, name string) error {
	key := strings.ToLower(name)
	if u[key] {
		return bad(path, "Такое название уже есть")
	}
	u[key] = true
	return nil
}

func validateRefs(f *File, st *sets) error {
	names := uniqueNames{}
	for i := range f.Categories {
		c := &f.Categories[i]
		if err := st.categories.add(at("categories", i, "id"), c.ID); err != nil {
			return err
		}
		name, err := catalog.CleanRefName("name", c.Name)
		if err != nil {
			return fromField(at("categories", i, ""), err)
		}
		c.Name = name
		if err := names.add(at("categories", i, "name"), name); err != nil {
			return err
		}
		if err := catalog.ValidateColor(c.Color); err != nil {
			return fromField(at("categories", i, ""), err)
		}
		if err := inRange(at("categories", i, "sort"), c.Sort, -maxSort, maxSort, "Порядок"); err != nil {
			return err
		}
	}
	names = uniqueNames{}
	for i := range f.Locations {
		l := &f.Locations[i]
		if err := st.locations.add(at("locations", i, "id"), l.ID); err != nil {
			return err
		}
		name, err := catalog.CleanRefName("name", l.Name)
		if err != nil {
			return fromField(at("locations", i, ""), err)
		}
		l.Name = name
		if err := names.add(at("locations", i, "name"), name); err != nil {
			return err
		}
		if err := checkOptTime(at("locations", i, "checked_at"), l.CheckedAt); err != nil {
			return err
		}
		if err := inRange(at("locations", i, "sort"), l.Sort, -maxSort, maxSort, "Порядок"); err != nil {
			return err
		}
	}
	names = uniqueNames{}
	for i := range f.Stores {
		s := &f.Stores[i]
		if err := st.stores.add(at("stores", i, "id"), s.ID); err != nil {
			return err
		}
		name, err := catalog.CleanRefName("name", s.Name)
		if err != nil {
			return fromField(at("stores", i, ""), err)
		}
		s.Name = name
		if err := names.add(at("stores", i, "name"), name); err != nil {
			return err
		}
		if s.URL, err = catalog.ValidateStoreURL(s.URL); err != nil {
			return fromField(at("stores", i, ""), err)
		}
	}
	names = uniqueNames{}
	for i := range f.Purposes {
		p := &f.Purposes[i]
		if err := st.purposes.add(at("purposes", i, "id"), p.ID); err != nil {
			return err
		}
		name, err := catalog.CleanRefName("name", p.Name)
		if err != nil {
			return fromField(at("purposes", i, ""), err)
		}
		p.Name = name
		if err := names.add(at("purposes", i, "name"), name); err != nil {
			return err
		}
		if err := inRange(at("purposes", i, "sort"), p.Sort, -maxSort, maxSort, "Порядок"); err != nil {
			return err
		}
	}
	return nil
}

// validateProductV2 проверяет поля позиции версии 2 (ФТ-15) и возвращает
// pH в десятых долях.
func validateProductV2(f *File, st *sets, i int) (*int64, error) {
	p := &f.Products[i]
	var phX10 *int64
	if p.PH != nil {
		x, err := catalog.PhX10(*p.PH)
		if err != nil {
			return nil, fromField(at("products", i, ""), err)
		}
		phX10 = &x
	}
	if len(p.PurposeIDs) > catalog.MaxProductPurposes {
		return nil, bad(at("products", i, "purpose_ids"), fmt.Sprintf("Назначений у позиции — не больше %d", catalog.MaxProductPurposes))
	}
	seen := map[int64]bool{}
	for j, id := range p.PurposeIDs {
		path := fmt.Sprintf("products[%d].purpose_ids[%d]", i, j)
		if err := st.purposes.has(path, id, "Назначения"); err != nil {
			return nil, err
		}
		if seen[id] {
			return nil, bad(path, "Назначение повторяется")
		}
		seen[id] = true
	}
	return phX10, nil
}

func validateProducts(f *File, st *sets) error {
	for i := range f.Products {
		p := &f.Products[i]
		if err := st.products.add(at("products", i, "id"), p.ID); err != nil {
			return err
		}
		if err := st.categories.has(at("products", i, "category_id"), p.CategoryID, "Категории"); err != nil {
			return err
		}
		phX10, err := validateProductV2(f, st, i)
		if err != nil {
			return err
		}
		in, err := catalog.ValidateProduct(catalog.ProductInput{
			CategoryID: p.CategoryID, Name: p.Name, Brand: p.Brand, Unit: p.Unit, PackageSize: p.PackageSize,
			Barcode: p.Barcode, MinStock: p.MinStock, Note: p.Note,
			PhX10: phX10, Incompatible: p.Incompatible, Instruction: p.Instruction, Rating: p.Rating,
		})
		if err != nil {
			return fromField(at("products", i, ""), err)
		}
		p.Name, p.Brand, p.Barcode, p.Note = in.Name, in.Brand, in.Barcode, in.Note
		p.Incompatible, p.Instruction = in.Incompatible, in.Instruction
		if err := checkTime(at("products", i, "created_at"), p.CreatedAt); err != nil {
			return err
		}
		if err := validatePhoto(at("products", i, "photo"), p.Photo); err != nil {
			return err
		}
	}
	return nil
}

// validatePhoto проверяет фото позиции так же, как PUT /api/products/{id}/photo:
// base64, не больше 400 КБ, тип по содержимому, размеры не больше 4096 px.
func validatePhoto(path string, p *Photo) error {
	if p == nil {
		return nil
	}
	// Предел на строку до декодирования: base64 раздувает размер на треть.
	if len(p.Data) > base64.StdEncoding.EncodedLen(photo.MaxBytes) {
		return bad(path, "Фото больше 400 КБ")
	}
	raw, err := base64.StdEncoding.DecodeString(p.Data)
	if err != nil {
		return bad(path+".data", "Фото — не base64")
	}
	info, err := photo.Inspect(raw)
	if err != nil {
		var ce *catalog.Error
		if errors.As(catalog.PhotoError(err), &ce) {
			return bad(path, ce.Message)
		}
		return bad(path, "Неверное фото")
	}
	p.raw, p.info = raw, info
	return nil
}

func validatePurchases(f *File, st *sets) error {
	for i := range f.Purchases {
		p := &f.Purchases[i]
		if err := st.stores.hasOpt(at("purchases", i, "store_id"), p.StoreID, "Магазина"); err != nil {
			return err
		}
		if _, err := time.Parse("2006-01-02", p.Date); err != nil {
			return bad(at("purchases", i, "date"), "Дата — в виде ГГГГ-ММ-ДД")
		}
		note, err := text(at("purchases", i, "note"), p.Note, 0, maxNote, "Заметка — не длиннее 500 символов")
		if err != nil {
			return err
		}
		p.Note = note
		if err := checkTime(at("purchases", i, "created_at"), p.CreatedAt); err != nil {
			return err
		}
		if len(p.Lines) < 1 || len(p.Lines) > maxLines {
			return bad(at("purchases", i, "lines"), "В покупке — от 1 до 30 строк")
		}
		for j, l := range p.Lines {
			path := fmt.Sprintf("purchases[%d].lines[%d]", i, j)
			if err := st.lines.add(path+".id", l.ID); err != nil {
				return err
			}
			if err := st.products.has(path+".product_id", l.ProductID, "Позиции"); err != nil {
				return err
			}
			if err := inRange(path+".packages", l.Packages, 1, maxPackages, "Количество упаковок"); err != nil {
				return err
			}
			if err := inRange(path+".price_per_package", l.PricePerPackage, 0, maxPrice, "Цена в копейках"); err != nil {
				return err
			}
			st.lineProduct[l.ID] = l.ProductID
		}
	}
	return nil
}

func validateUnits(f *File, st *sets) error {
	for i := range f.Units {
		u := &f.Units[i]
		p := func(field string) string { return at("units", i, field) }
		if err := st.units.add(p("id"), u.ID); err != nil {
			return err
		}
		if err := st.products.has(p("product_id"), u.ProductID, "Позиции"); err != nil {
			return err
		}
		if u.PurchaseLineID != nil {
			if err := st.lines.has(p("purchase_line_id"), *u.PurchaseLineID, "Строки покупки"); err != nil {
				return err
			}
			if st.lineProduct[*u.PurchaseLineID] != u.ProductID {
				return bad(p("purchase_line_id"), "Строка покупки относится к другой позиции")
			}
		}
		if err := st.locations.hasOpt(p("location_id"), u.LocationID, "Места"); err != nil {
			return err
		}
		if err := inRange(p("initial_qty"), u.InitialQty, 1, maxQty, "Объём"); err != nil {
			return err
		}
		if err := inRange(p("remaining_qty"), u.RemainingQty, 0, u.InitialQty, "Остаток"); err != nil {
			return err
		}
		switch u.Status {
		case "sealed", "open", "finished", "written_off":
		default:
			return bad(p("status"), "Статус — sealed, open, finished или written_off")
		}
		if err := checkUnitState(p, u); err != nil {
			return err
		}
		for field, v := range map[string]*int64{"opened_at": u.OpenedAt, "finished_at": u.FinishedAt} {
			if err := checkOptTime(p(field), v); err != nil {
				return err
			}
		}
		if err := checkTime(p("created_at"), u.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func validateBalances(f *File, _ *sets) error { return checkBalances(f) }

func validateRecipes(f *File, st *sets) error {
	names := uniqueNames{}
	recipes := ids{}
	for i := range f.Recipes {
		r := &f.Recipes[i]
		if err := recipes.add(at("recipes", i, "id"), r.ID); err != nil {
			return err
		}
		name, err := text(at("recipes", i, "name"), r.Name, 1, maxRecipeName, "Название — от 1 до 60 символов")
		if err != nil {
			return err
		}
		r.Name = name
		if err := names.add(at("recipes", i, "name"), name); err != nil {
			return err
		}
		if err := checkTime(at("recipes", i, "created_at"), r.CreatedAt); err != nil {
			return err
		}
		if len(r.Items) < 1 || len(r.Items) > maxLines {
			return bad(at("recipes", i, "items"), "В шаблоне — от 1 до 30 строк")
		}
		seen := ids{}
		for j, it := range r.Items {
			path := fmt.Sprintf("recipes[%d].items[%d]", i, j)
			if err := st.products.has(path+".product_id", it.ProductID, "Позиции"); err != nil {
				return err
			}
			if seen[it.ProductID] {
				return bad(path+".product_id", "Позиция повторяется в нескольких строках")
			}
			seen[it.ProductID] = true
			if err := inRange(path+".qty", it.Qty, 1, maxQty, "Количество"); err != nil {
				return err
			}
		}
	}
	for i, run := range f.RecipeRuns {
		if err := st.runs.add(at("recipe_runs", i, "id"), run.ID); err != nil {
			return err
		}
		if err := checkTime(at("recipe_runs", i, "created_at"), run.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func validateMovements(f *File, st *sets) error {
	for i := range f.Movements {
		m := &f.Movements[i]
		if err := st.movements.add(at("movements", i, "id"), m.ID); err != nil {
			return err
		}
	}
	for i := range f.Movements {
		m := &f.Movements[i]
		p := func(field string) string { return at("movements", i, field) }
		if err := st.units.has(p("unit_id"), m.UnitID, "Экземпляра"); err != nil {
			return err
		}
		if err := inRange(p("qty_delta"), m.QtyDelta, -maxQty, maxQty, "Изменение остатка"); err != nil {
			return err
		}
		okSign := map[string]bool{
			"purchase": m.QtyDelta > 0, "consume": m.QtyDelta < 0, "writeoff": m.QtyDelta < 0,
			"move": m.QtyDelta == 0, "adjust": true,
		}
		ok, known := okSign[m.Type]
		if !known {
			return bad(p("type"), "Тип — purchase, consume, writeoff, move или adjust")
		}
		if !ok {
			return bad(p("qty_delta"), "Знак изменения не подходит к типу движения")
		}
		for field, id := range map[string]*int64{"from_location_id": m.FromLocationID, "to_location_id": m.ToLocationID} {
			if err := st.locations.hasOpt(p(field), id, "Места"); err != nil {
				return err
			}
		}
		if err := st.runs.hasOpt(p("recipe_run_id"), m.RecipeRunID, "Применения шаблона"); err != nil {
			return err
		}
		if err := st.movements.hasOpt(p("batch_id"), m.BatchID, "Движения"); err != nil {
			return err
		}
		reason, err := text(p("reason"), m.Reason, 0, maxReason, "Причина — не длиннее 80 символов")
		if err != nil {
			return err
		}
		m.Reason = reason
		if err := checkTime(p("created_at"), m.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func validateShopping(f *File, st *sets) error {
	seen := ids{}
	for i := range f.Shopping {
		it := &f.Shopping[i]
		p := func(field string) string { return at("shopping", i, field) }
		if err := seen.add(p("id"), it.ID); err != nil {
			return err
		}
		if err := st.products.hasOpt(p("product_id"), it.ProductID, "Позиции"); err != nil {
			return err
		}
		title, err := catalog.CleanShoppingTitle(it.Title)
		if err != nil {
			return fromField(at("shopping", i, ""), err)
		}
		it.Title = title
		if it.ProductID == nil && title == "" {
			return bad(p("title"), "Укажите позицию или название")
		}
		if err := checkOptTime(p("done_at"), it.DoneAt); err != nil {
			return err
		}
		if err := checkTime(p("created_at"), it.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}
