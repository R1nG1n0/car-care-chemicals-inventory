package stock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// RecipeInput — тело POST и PUT /api/recipes.
type RecipeInput struct {
	Name  string      `json:"name"`
	Items []ItemInput `json:"items"`
}

// ItemInput — строка «позиция — количество». Вместо qty можно передать
// Solution (ФТ-17.2.4): тогда qty игнорируется и считается из раствора.
type ItemInput struct {
	ProductID int64     `json:"product_id"`
	Qty       int64     `json:"qty"`
	Solution  *Solution `json:"solution"`
}

// Recipe — шаблон расхода. Сводку позиции к строкам добавляет обработчик.
type Recipe struct {
	ID         int64
	Name       string
	Sort       int64
	LastUsedAt *int64
	Cost       *int64 // оценка по ценам экземпляров, из которых пойдёт списание
	Items      []RecipeItem
}

// RecipeItem — строка шаблона с доступным остатком.
type RecipeItem struct {
	ProductID int64
	Qty       int64     // для строки-раствора — рассчитанный концентрат
	Solution  *Solution // nil — строка задана количеством
	Available int64
	Short     bool
}

// validateItems проверяет строки: позиции свои и без повторов, qty в
// пределах; zeroOK — допускается qty 0 (применение: строка пропускается).
// У строки с раствором qty заменяется рассчитанным концентратом (в items).
func validateItems(ctx context.Context, q querier, userID int64, items []ItemInput, zeroOK bool) ([]productInfo, error) {
	if len(items) > maxLines || (!zeroOK && len(items) < 1) {
		return nil, &ValidationError{Field: "items", Message: "В шаблоне — от 1 до 30 строк"}
	}
	minQty := int64(1)
	if zeroOK {
		minQty = 0
	}
	seen := map[int64]bool{}
	out := make([]productInfo, 0, len(items))
	for i := range items {
		it := &items[i]
		if it.Solution == nil && (it.Qty < minQty || it.Qty > maxQty) {
			return nil, &ValidationError{Field: "qty", Message: fmt.Sprintf("Количество — от %d до 100 000", minQty)}
		}
		if seen[it.ProductID] {
			return nil, &ValidationError{Field: "items", Message: "Позиция повторяется в нескольких строках"}
		}
		seen[it.ProductID] = true
		p, err := product(ctx, q, userID, it.ProductID)
		if err != nil {
			return nil, err
		}
		if it.Solution != nil {
			if it.Qty, err = it.Solution.resolve(p); err != nil {
				return nil, err
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func validRecipeName(name string) (string, error) {
	return cleanText(name, 1, maxRecipeName, "name", "Название — от 1 до 60 символов")
}

var errRecipeNameTaken = &ConflictError{Field: "name", Message: "Шаблон с таким названием уже есть"}

// recipeNameFree проверяет, что у пользователя нет другого шаблона (кроме
// exceptID) с тем же именем без учёта регистра: «Мойка Кузова» и «мойка
// кузова» — дубль. LOWER и COLLATE NOCASE в SQLite знают только латиницу,
// поэтому сравнение — в Go, в той же транзакции, что и запись.
func recipeNameFree(ctx context.Context, q querier, userID, exceptID int64, name string) error {
	rows, err := q.QueryContext(ctx, `SELECT name FROM recipes WHERE user_id = ? AND id <> ?`, userID, exceptID)
	if err != nil {
		return fmt.Errorf("проверка названия шаблона: %w", err)
	}
	defer rows.Close()
	key := strings.ToLower(name)
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return fmt.Errorf("проверка названия шаблона: %w", err)
		}
		if strings.ToLower(n) == key {
			return errRecipeNameTaken
		}
	}
	return rows.Err()
}

// CreateRecipe создаёт шаблон в конце списка.
func (s *Service) CreateRecipe(ctx context.Context, userID int64, in RecipeInput) (int64, error) {
	name, err := validRecipeName(in.Name)
	if err != nil {
		return 0, err
	}
	now := s.now().Unix()
	var id int64
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		if _, err := validateItems(ctx, tx, userID, in.Items, false); err != nil {
			return err
		}
		if err := recipeNameFree(ctx, tx, userID, 0, name); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO recipes (user_id, name, sort, created_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(sort), -1) + 1 FROM recipes WHERE user_id = ?), ?)`,
			userID, name, userID, now)
		if isUniqueViolation(err) {
			return errRecipeNameTaken
		}
		if err != nil {
			return fmt.Errorf("запись шаблона: %w", err)
		}
		if id, err = res.LastInsertId(); err != nil {
			return err
		}
		return insertItems(ctx, tx, userID, id, in.Items)
	})
	return id, err
}

func insertItems(ctx context.Context, tx *sql.Tx, userID, recipeID int64, items []ItemInput) error {
	for i, it := range items {
		ml, ratio := it.Solution.nulls()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO recipe_items (user_id, recipe_id, product_id, qty, sort, solution_ml, solution_ratio)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID, recipeID, it.ProductID, it.Qty, i, ml, ratio); err != nil {
			return fmt.Errorf("запись строки шаблона: %w", err)
		}
	}
	return nil
}

func recipeExists(ctx context.Context, q querier, userID, id int64) error {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM recipes WHERE user_id = ? AND id = ?`, userID, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return errRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("чтение шаблона: %w", err)
	}
	return nil
}

// UpdateRecipe заменяет имя и строки шаблона.
func (s *Service) UpdateRecipe(ctx context.Context, userID, id int64, in RecipeInput) error {
	name, err := validRecipeName(in.Name)
	if err != nil {
		return err
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := recipeExists(ctx, tx, userID, id); err != nil {
			return err
		}
		if _, err := validateItems(ctx, tx, userID, in.Items, false); err != nil {
			return err
		}
		if err := recipeNameFree(ctx, tx, userID, id, name); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE recipes SET name = ? WHERE user_id = ? AND id = ?`, name, userID, id)
		if isUniqueViolation(err) {
			return errRecipeNameTaken
		}
		if err != nil {
			return fmt.Errorf("изменение шаблона: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM recipe_items WHERE user_id = ? AND recipe_id = ?`,
			userID, id); err != nil {
			return fmt.Errorf("удаление строк шаблона: %w", err)
		}
		return insertItems(ctx, tx, userID, id, in.Items)
	})
}

// DeleteRecipe удаляет шаблон; журнал и записи применений остаются.
func (s *Service) DeleteRecipe(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM recipes WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return fmt.Errorf("удаление шаблона: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errRecipeNotFound
	}
	return nil
}

// ReorderRecipes задаёт порядок: ids — ровно все шаблоны пользователя.
func (s *Service) ReorderRecipes(ctx context.Context, userID int64, ids []int64) error {
	bad := &ValidationError{Field: "ids", Message: "Порядок должен содержать все шаблоны ровно по одному разу"}
	if len(uniqueIDs(ids)) != len(ids) {
		return bad
	}
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM recipes WHERE user_id = ?`, userID).Scan(&n); err != nil {
			return fmt.Errorf("подсчёт шаблонов: %w", err)
		}
		if n != len(ids) {
			return bad
		}
		for i, id := range ids {
			res, err := tx.ExecContext(ctx, `UPDATE recipes SET sort = ? WHERE user_id = ? AND id = ?`, i, userID, id)
			if err != nil {
				return fmt.Errorf("порядок шаблонов: %w", err)
			}
			if k, _ := res.RowsAffected(); k == 0 {
				return bad
			}
		}
		return nil
	})
}

// Recipes — все шаблоны по sort; id 0 — все, иначе один (нет — 404).
func (s *Service) Recipes(ctx context.Context, userID, id int64) ([]Recipe, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.name, r.sort,
		       (SELECT MAX(created_at) FROM recipe_runs WHERE user_id = r.user_id AND recipe_id = r.id)
		FROM recipes r WHERE r.user_id = ? AND (? = 0 OR r.id = ?)
		ORDER BY r.sort, r.id`, userID, id, id)
	if err != nil {
		return nil, fmt.Errorf("список шаблонов: %w", err)
	}
	defer rows.Close()
	out := []Recipe{}
	for rows.Next() {
		r := Recipe{Items: []RecipeItem{}}
		var last sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.Sort, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			r.LastUsedAt = &last.Int64
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if id != 0 && len(out) == 0 {
		return nil, errRecipeNotFound
	}
	for i := range out {
		if err := s.fillItems(ctx, userID, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// fillItems читает строки шаблона, остатки и оценку себестоимости.
func (s *Service) fillItems(ctx context.Context, userID int64, r *Recipe) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.product_id, i.qty, p.unit, p.package_size, p.name, i.solution_ml, i.solution_ratio FROM recipe_items i
		JOIN products p ON p.user_id = i.user_id AND p.id = i.product_id
		WHERE i.user_id = ? AND i.recipe_id = ? ORDER BY i.sort, i.id`, userID, r.ID)
	if err != nil {
		return fmt.Errorf("строки шаблона: %w", err)
	}
	defer rows.Close()
	var infos []productInfo
	for rows.Next() {
		var it RecipeItem
		var p productInfo
		var solML, solRatio sql.NullInt64
		if err := rows.Scan(&it.ProductID, &it.Qty, &p.Unit, &p.PackageSize, &p.Name, &solML, &solRatio); err != nil {
			return err
		}
		it.Solution = solutionFrom(solML, solRatio)
		p.ID = it.ProductID
		r.Items = append(r.Items, it)
		infos = append(infos, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	for i := range r.Items {
		pl, err := planConsume(ctx, s.db, userID, infos[i], r.Items[i].Qty, nil)
		if err != nil {
			return err
		}
		r.Items[i].Available = pl.available
		r.Items[i].Short = pl.available < r.Items[i].Qty
		r.Cost = addCost(r.Cost, pl.cost)
	}
	return nil
}

// Shortage — строка шаблона, которой не хватило (ФТ-8.6).
type Shortage struct {
	ProductID int64 `json:"product_id"`
	Requested int64 `json:"requested"`
	Consumed  int64 `json:"consumed"`
}

// ApplyResult — итог применения шаблона.
type ApplyResult struct {
	RunID       int64
	MovementIDs []int64
	ProductIDs  []int64
	Shortages   []Shortage
}

// ApplyRecipe применяет шаблон с поправленными количествами одной
// транзакцией: запись recipe_runs и расход по правилам Consume. qty 0 —
// строка пропускается; нехватка не ошибка — списывается сколько есть.
func (s *Service) ApplyRecipe(ctx context.Context, userID, recipeID int64, items []ItemInput) (ApplyResult, error) {
	res, _, _, err := s.ApplyRecipeOp(ctx, userID, recipeID, items, Op{}, nil)
	return res, err
}

// ApplyRecipeOp — ApplyRecipe с номером операции и временем с телефона
// (ФТ-12.4); правила — как у ConsumeOp.
func (s *Service) ApplyRecipeOp(ctx context.Context, userID, recipeID int64, items []ItemInput, op Op,
	render Render[ApplyResult]) (ApplyResult, []byte, bool, error) {
	return runOp(ctx, s, userID, op, opKindApply, func(tx *sql.Tx, now int64) (ApplyResult, error) {
		res := ApplyResult{MovementIDs: []int64{}, ProductIDs: []int64{}, Shortages: []Shortage{}}
		if err := recipeExists(ctx, tx, userID, recipeID); err != nil {
			return res, err
		}
		infos, err := validateItems(ctx, tx, userID, items, true)
		if err != nil {
			return res, err
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO recipe_runs (user_id, recipe_id, created_at) VALUES (?, ?, ?)`,
			userID, recipeID, now)
		if err != nil {
			return res, fmt.Errorf("запись применения шаблона: %w", err)
		}
		if res.RunID, err = r.LastInsertId(); err != nil {
			return res, err
		}
		run := sql.NullInt64{Int64: res.RunID, Valid: true}
		var batch int64 // все движения применения — одна группа
		for i, it := range items {
			if it.Qty == 0 {
				continue
			}
			c, err := consumeTx(ctx, tx, userID, infos[i], it.Qty, nil, run, &batch, now, true, it.Solution)
			if err != nil {
				return res, err
			}
			res.MovementIDs = append(res.MovementIDs, c.MovementIDs...)
			res.ProductIDs = append(res.ProductIDs, it.ProductID)
			if c.Consumed < it.Qty {
				res.Shortages = append(res.Shortages, Shortage{ProductID: it.ProductID, Requested: it.Qty, Consumed: c.Consumed})
			}
		}
		if len(res.ProductIDs) == 0 {
			return res, &ValidationError{Field: "items", Message: "Нечего списывать: все количества равны нулю"}
		}
		return res, nil
	}, render)
}
