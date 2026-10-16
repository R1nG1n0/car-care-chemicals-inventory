package stock

import (
	"testing"
	"time"
)

func TestRecipeCRUDAndApply(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	shampoo := e.stkProduct(uid, cat, "Шампунь", "ml", 1000)
	foam := e.stkProduct(uid, cat, "Пена", "ml", 500)
	towel := e.stkProduct(uid, cat, "Салфетки", "pcs", 10)
	e.stkBuy(uid, shampoo, "2026-10-01", 1, 100000, nil) // 100 коп. за мл
	e.stkBuy(uid, foam, "2026-10-01", 1, 50000, nil)     // 100 коп. за мл
	e.stkBuy(uid, towel, "2026-10-01", 1, 1000, nil)     // 100 коп. за шт

	id, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Мойка", Items: []ItemInput{
		{ProductID: shampoo, Qty: 40}, {ProductID: foam, Qty: 600}, {ProductID: towel, Qty: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	list, err := e.svc.Recipes(e.ctx, uid, id)
	if err != nil {
		t.Fatal(err)
	}
	r := list[0]
	if r.Name != "Мойка" || len(r.Items) != 3 || r.LastUsedAt != nil {
		t.Fatalf("шаблон: %+v", r)
	}
	// Пены 500 из 600: строка помечена, себестоимость — по тому, что есть.
	if !r.Items[1].Short || r.Items[1].Available != 500 || r.Items[0].Short {
		t.Fatalf("строки: %+v", r.Items)
	}
	if r.Cost == nil || *r.Cost != 4000+50000+200 {
		t.Fatalf("оценка себестоимости %v", r.Cost)
	}

	// Проверки ввода.
	_, err = e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Мойка", Items: []ItemInput{{ProductID: shampoo, Qty: 1}}})
	if ce := stkIs[*ConflictError](t, err, "повтор имени"); ce.Field != "name" {
		t.Fatalf("поле %q", ce.Field)
	}
	_, err = e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Дубль", Items: []ItemInput{
		{ProductID: shampoo, Qty: 1}, {ProductID: shampoo, Qty: 2}}})
	stkIs[*ValidationError](t, err, "повтор позиции")
	_, err = e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Пусто"})
	stkIs[*ValidationError](t, err, "без строк")

	// Применение с поправленными количествами: шампунь 50, салфетки
	// пропущены, пены не хватает — списывается сколько есть.
	e.clock.Add(time.Hour)
	res, err := e.svc.ApplyRecipe(e.ctx, uid, id, []ItemInput{
		{ProductID: shampoo, Qty: 50}, {ProductID: foam, Qty: 600}, {ProductID: towel, Qty: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Shortages) != 1 || res.Shortages[0] != (Shortage{ProductID: foam, Requested: 600, Consumed: 500}) {
		t.Fatalf("нехватка: %+v", res.Shortages)
	}
	if len(res.MovementIDs) != 2 || len(res.ProductIDs) != 2 {
		t.Fatalf("применение: %+v", res)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE recipe_run_id = ?`, res.RunID); n != 2 {
		t.Fatalf("движений с run_id: %d", n)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM units WHERE product_id = ? AND status = 'sealed'`, towel); n != 1 {
		t.Fatal("строка с qty 0 не пропущена")
	}
	list, _ = e.svc.Recipes(e.ctx, uid, id)
	if list[0].LastUsedAt == nil || *list[0].LastUsedAt != e.clock.Now().Unix() {
		t.Fatalf("last_used_at: %v", list[0].LastUsedAt)
	}

	// Отмена применения через revert возвращает всё.
	if _, err := e.svc.Revert(e.ctx, uid, res.MovementIDs); err != nil {
		t.Fatal(err)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM units WHERE user_id = ? AND status = 'sealed'`, uid); n != 3 {
		t.Fatalf("после отмены запечатанных %d", n)
	}

	// Все нули — нечего списывать; шаблон не изменился.
	_, err = e.svc.ApplyRecipe(e.ctx, uid, id, []ItemInput{{ProductID: shampoo, Qty: 0}})
	stkIs[*ValidationError](t, err, "все нули")

	// Замена, порядок, удаление.
	id2, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Стёкла", Items: []ItemInput{{ProductID: towel, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.UpdateRecipe(e.ctx, uid, id, RecipeInput{Name: "Мойка кузова", Items: []ItemInput{{ProductID: foam, Qty: 30}}}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.ReorderRecipes(e.ctx, uid, []int64{id2, id}); err != nil {
		t.Fatal(err)
	}
	stkIs[*ValidationError](t, e.svc.ReorderRecipes(e.ctx, uid, []int64{id2}), "неполный порядок")
	all, _ := e.svc.Recipes(e.ctx, uid, 0)
	if len(all) != 2 || all[0].ID != id2 || all[1].Name != "Мойка кузова" || len(all[1].Items) != 1 {
		t.Fatalf("после замены и порядка: %+v", all)
	}
	if err := e.svc.DeleteRecipe(e.ctx, uid, id); err != nil {
		t.Fatal(err)
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'consume'`); n != 0 {
		t.Fatalf("движения: %d", n)
	}
	if !IsNotFound(e.svc.DeleteRecipe(e.ctx, uid, id)) {
		t.Fatal("повторное удаление")
	}
	if _, err := e.svc.Recipes(e.ctx, uid, id); !IsNotFound(err) {
		t.Fatalf("удалённый шаблон: %v", err)
	}
}

func TestRecipeIsolation(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	uid2, cat2 := e.stkUser("b")
	mine := e.stkProduct(uid, cat, "Моя", "ml", 100)
	foreign := e.stkProduct(uid2, cat2, "Чужая", "ml", 100)
	id, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "Мойка", Items: []ItemInput{{ProductID: mine, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.CreateRecipe(e.ctx, uid2, RecipeInput{Name: "X", Items: []ItemInput{{ProductID: mine, Qty: 1}}}); !IsNotFound(err) {
		t.Fatalf("чужая позиция в шаблоне: %v", err)
	}
	if _, err := e.svc.ApplyRecipe(e.ctx, uid2, id, []ItemInput{{ProductID: foreign, Qty: 1}}); !IsNotFound(err) {
		t.Fatalf("применение чужого шаблона: %v", err)
	}
	if _, err := e.svc.ApplyRecipe(e.ctx, uid, id, []ItemInput{{ProductID: foreign, Qty: 1}}); !IsNotFound(err) {
		t.Fatalf("чужая позиция при применении: %v", err)
	}
	if err := e.svc.UpdateRecipe(e.ctx, uid2, id, RecipeInput{Name: "X", Items: []ItemInput{{ProductID: foreign, Qty: 1}}}); !IsNotFound(err) {
		t.Fatalf("изменение чужого шаблона: %v", err)
	}
	if !IsNotFound(e.svc.DeleteRecipe(e.ctx, uid2, id)) {
		t.Fatal("удаление чужого шаблона")
	}
	if n := e.stkCount(`SELECT COUNT(*) FROM recipe_runs`); n != 0 {
		t.Fatalf("записи применений: %d", n)
	}
}
