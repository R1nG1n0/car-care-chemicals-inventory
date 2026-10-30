package transfer

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"
)

// Service — экспорт и импорт данных пользователя.
type Service struct {
	db  *sql.DB
	now func() time.Time
	// importing — идёт импорт (BeginImport): одновременно не больше одного.
	importing atomic.Bool
}

// New создаёт сервис; now == nil — time.Now.
func New(db *sql.DB, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

type querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// each выполняет запрос и вызывает scan для каждой строки.
func each(ctx context.Context, q querier, what, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("экспорт, %s: %w", what, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("экспорт, %s: %w", what, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("экспорт, %s: %w", what, err)
	}
	return nil
}

func ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	n := v.Int64
	return &n
}

// Export собирает данные пользователя одним согласованным снимком, кроме фото
// позиций: они тяжёлые и читаются отдельно (WriteJSON). Пароль, сессии и
// приглашения в файл не входят.
func (s *Service) Export(ctx context.Context, userID int64) (*File, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("экспорт: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // чтение: откат — no-op

	f := &File{
		Format:     FormatName,
		Version:    FormatVersion,
		ExportedAt: s.now().UTC().Format(time.RFC3339),
		Categories: []Category{}, Locations: []Location{}, Stores: []Store{}, Purposes: []Purpose{}, Products: []Product{},
		Units: []Unit{}, Purchases: []Purchase{}, RecipeRuns: []RecipeRun{}, Movements: []Movement{},
		Recipes: []Recipe{}, Shopping: []Shopping{},
	}
	if err := tx.QueryRowContext(ctx, `SELECT login FROM users WHERE id = ?`, userID).Scan(&f.User.Login); err != nil {
		return nil, fmt.Errorf("экспорт, пользователь: %w", err)
	}
	steps := []func(context.Context, querier, int64, *File) error{
		exportSettings, exportRefs, exportProducts, exportPurchases, exportUnits,
		exportMovements, exportRecipes, exportShopping,
	}
	for _, step := range steps {
		if err := step(ctx, tx, userID, f); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func exportSettings(ctx context.Context, q querier, uid int64, f *File) error {
	var st Settings
	err := q.QueryRowContext(ctx, `SELECT step_small, step_large FROM user_settings WHERE user_id = ?`, uid).
		Scan(&st.StepSmall, &st.StepLarge)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("экспорт, настройки: %w", err)
	}
	f.Settings = &st
	return nil
}

func exportRefs(ctx context.Context, q querier, uid int64, f *File) error {
	u := []any{uid}
	err := each(ctx, q, "категории", `SELECT id, name, color, sort, archived FROM categories WHERE user_id = ? ORDER BY id`, u,
		func(r *sql.Rows) error {
			var c Category
			if err := r.Scan(&c.ID, &c.Name, &c.Color, &c.Sort, &c.Archived); err != nil {
				return err
			}
			f.Categories = append(f.Categories, c)
			return nil
		})
	if err != nil {
		return err
	}
	err = each(ctx, q, "места", `SELECT id, name, sort, archived, checked_at FROM locations WHERE user_id = ? ORDER BY id`, u,
		func(r *sql.Rows) error {
			var l Location
			var checked sql.NullInt64
			if err := r.Scan(&l.ID, &l.Name, &l.Sort, &l.Archived, &checked); err != nil {
				return err
			}
			l.CheckedAt = ptr(checked)
			f.Locations = append(f.Locations, l)
			return nil
		})
	if err != nil {
		return err
	}
	err = each(ctx, q, "магазины", `SELECT id, name, url, archived FROM stores WHERE user_id = ? ORDER BY id`, u,
		func(r *sql.Rows) error {
			var st Store
			if err := r.Scan(&st.ID, &st.Name, &st.URL, &st.Archived); err != nil {
				return err
			}
			f.Stores = append(f.Stores, st)
			return nil
		})
	if err != nil {
		return err
	}
	return each(ctx, q, "назначения", `SELECT id, name, sort, archived FROM purposes WHERE user_id = ? ORDER BY id`, u,
		func(r *sql.Rows) error {
			var p Purpose
			if err := r.Scan(&p.ID, &p.Name, &p.Sort, &p.Archived); err != nil {
				return err
			}
			f.Purposes = append(f.Purposes, p)
			return nil
		})
}

func exportProducts(ctx context.Context, q querier, uid int64, f *File) error {
	err := each(ctx, q, "позиции", `SELECT id, category_id, name, brand, unit, package_size, barcode, min_stock,
		note, archived, created_at, ph_x10, incompatible, instruction, rating, do_not_buy
		FROM products WHERE user_id = ? ORDER BY id`, []any{uid},
		func(r *sql.Rows) error {
			var p Product
			var minStock, phX10, rating sql.NullInt64
			if err := r.Scan(&p.ID, &p.CategoryID, &p.Name, &p.Brand, &p.Unit, &p.PackageSize, &p.Barcode,
				&minStock, &p.Note, &p.Archived, &p.CreatedAt, &phX10, &p.Incompatible, &p.Instruction,
				&rating, &p.DoNotBuy); err != nil {
				return err
			}
			p.MinStock, p.Rating = ptr(minStock), ptr(rating)
			if phX10.Valid {
				v := float64(phX10.Int64) / 10
				p.PH = &v
			}
			p.PurposeIDs = []int64{}
			p.Dilutions = []Dilution{}
			f.Products = append(f.Products, p)
			return nil
		})
	if err != nil {
		return err
	}
	byID := make(map[int64]int, len(f.Products))
	for i, p := range f.Products {
		byID[p.ID] = i
	}
	err = each(ctx, q, "пропорции позиций", `SELECT product_id, name, ratio FROM product_dilutions
		WHERE user_id = ? ORDER BY product_id, sort, id`, []any{uid}, func(r *sql.Rows) error {
		var product int64
		var d Dilution
		if err := r.Scan(&product, &d.Name, &d.Ratio); err != nil {
			return err
		}
		i, ok := byID[product]
		if !ok {
			return fmt.Errorf("пропорция: нет позиции %d", product)
		}
		f.Products[i].Dilutions = append(f.Products[i].Dilutions, d)
		return nil
	})
	if err != nil {
		return err
	}
	return each(ctx, q, "назначения позиций", `SELECT product_id, purpose_id FROM product_purposes
		WHERE user_id = ? ORDER BY product_id, purpose_id`, []any{uid}, func(r *sql.Rows) error {
		var product, purpose int64
		if err := r.Scan(&product, &purpose); err != nil {
			return err
		}
		i, ok := byID[product]
		if !ok {
			return fmt.Errorf("назначение %d: нет позиции %d", purpose, product)
		}
		f.Products[i].PurposeIDs = append(f.Products[i].PurposeIDs, purpose)
		return nil
	})
}

func exportPurchases(ctx context.Context, q querier, uid int64, f *File) error {
	byID := map[int64]int{}
	err := each(ctx, q, "покупки", `SELECT id, store_id, date, note, created_at FROM purchases
		WHERE user_id = ? ORDER BY id`, []any{uid}, func(r *sql.Rows) error {
		p := Purchase{Lines: []PurchaseLine{}}
		var store sql.NullInt64
		if err := r.Scan(&p.ID, &store, &p.Date, &p.Note, &p.CreatedAt); err != nil {
			return err
		}
		p.StoreID = ptr(store)
		byID[p.ID] = len(f.Purchases)
		f.Purchases = append(f.Purchases, p)
		return nil
	})
	if err != nil {
		return err
	}
	return each(ctx, q, "строки покупок", `SELECT id, purchase_id, product_id, packages, price_per_package
		FROM purchase_lines WHERE user_id = ? ORDER BY id`, []any{uid}, func(r *sql.Rows) error {
		var l PurchaseLine
		var purchase int64
		if err := r.Scan(&l.ID, &purchase, &l.ProductID, &l.Packages, &l.PricePerPackage); err != nil {
			return err
		}
		i, ok := byID[purchase]
		if !ok {
			return fmt.Errorf("строка %d: нет покупки %d", l.ID, purchase)
		}
		f.Purchases[i].Lines = append(f.Purchases[i].Lines, l)
		return nil
	})
}

func exportUnits(ctx context.Context, q querier, uid int64, f *File) error {
	return each(ctx, q, "экземпляры", `SELECT id, product_id, purchase_line_id, location_id, initial_qty,
		remaining_qty, status, opened_at, finished_at, created_at FROM units WHERE user_id = ? ORDER BY id`,
		[]any{uid}, func(r *sql.Rows) error {
			var u Unit
			var line, loc, opened, finished sql.NullInt64
			if err := r.Scan(&u.ID, &u.ProductID, &line, &loc, &u.InitialQty, &u.RemainingQty, &u.Status,
				&opened, &finished, &u.CreatedAt); err != nil {
				return err
			}
			u.PurchaseLineID, u.LocationID, u.OpenedAt, u.FinishedAt = ptr(line), ptr(loc), ptr(opened), ptr(finished)
			f.Units = append(f.Units, u)
			return nil
		})
}

func exportMovements(ctx context.Context, q querier, uid int64, f *File) error {
	err := each(ctx, q, "применения шаблонов", `SELECT id, recipe_id, created_at FROM recipe_runs
		WHERE user_id = ? ORDER BY id`, []any{uid}, func(r *sql.Rows) error {
		var run RecipeRun
		var recipe sql.NullInt64
		if err := r.Scan(&run.ID, &recipe, &run.CreatedAt); err != nil {
			return err
		}
		run.RecipeID = ptr(recipe)
		f.RecipeRuns = append(f.RecipeRuns, run)
		return nil
	})
	if err != nil {
		return err
	}
	return each(ctx, q, "движения", `SELECT id, unit_id, type, qty_delta, from_location_id, to_location_id,
		reason, recipe_run_id, batch_id, created_at, solution_ml, solution_ratio
		FROM movements WHERE user_id = ? ORDER BY id`,
		[]any{uid}, func(r *sql.Rows) error {
			var m Movement
			var from, to, run, batch, solML, solRatio sql.NullInt64
			if err := r.Scan(&m.ID, &m.UnitID, &m.Type, &m.QtyDelta, &from, &to, &m.Reason, &run, &batch,
				&m.CreatedAt, &solML, &solRatio); err != nil {
				return err
			}
			m.FromLocationID, m.ToLocationID, m.RecipeRunID, m.BatchID = ptr(from), ptr(to), ptr(run), ptr(batch)
			m.SolutionML, m.SolutionRatio = ptr(solML), ptr(solRatio)
			f.Movements = append(f.Movements, m)
			return nil
		})
}

func exportRecipes(ctx context.Context, q querier, uid int64, f *File) error {
	byID := map[int64]int{}
	err := each(ctx, q, "шаблоны", `SELECT id, name, sort, created_at FROM recipes WHERE user_id = ? ORDER BY id`,
		[]any{uid}, func(r *sql.Rows) error {
			rc := Recipe{Items: []RecipeItem{}}
			if err := r.Scan(&rc.ID, &rc.Name, &rc.Sort, &rc.CreatedAt); err != nil {
				return err
			}
			byID[rc.ID] = len(f.Recipes)
			f.Recipes = append(f.Recipes, rc)
			return nil
		})
	if err != nil {
		return err
	}
	return each(ctx, q, "строки шаблонов", `SELECT recipe_id, product_id, qty, sort, solution_ml, solution_ratio FROM recipe_items
		WHERE user_id = ? ORDER BY recipe_id, sort, id`, []any{uid}, func(r *sql.Rows) error {
		var it RecipeItem
		var recipe int64
		var solML, solRatio sql.NullInt64
		if err := r.Scan(&recipe, &it.ProductID, &it.Qty, &it.Sort, &solML, &solRatio); err != nil {
			return err
		}
		it.SolutionML, it.SolutionRatio = ptr(solML), ptr(solRatio)
		i, ok := byID[recipe]
		if !ok {
			return fmt.Errorf("строка шаблона: нет шаблона %d", recipe)
		}
		f.Recipes[i].Items = append(f.Recipes[i].Items, it)
		return nil
	})
}

func exportShopping(ctx context.Context, q querier, uid int64, f *File) error {
	return each(ctx, q, "список покупок", `SELECT id, product_id, title, done_at, created_at FROM shopping_items
		WHERE user_id = ? ORDER BY id`, []any{uid}, func(r *sql.Rows) error {
		var it Shopping
		var product, done sql.NullInt64
		if err := r.Scan(&it.ID, &product, &it.Title, &done, &it.CreatedAt); err != nil {
			return err
		}
		it.ProductID, it.DoneAt = ptr(product), ptr(done)
		f.Shopping = append(f.Shopping, it)
		return nil
	})
}
