package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
)

// Ресурс микрофибр и губок (ФТ-19): API целиком.

type ragStarted struct {
	Rag         stock.Rag              `json:"rag"`
	Product     catalog.ProductSummary `json:"product"`
	MovementIDs []int64                `json:"movement_ids"`
}

type ragList struct {
	Rags []stock.Rag `json:"rags"`
}

func ragProduct(c *client, cat, limit int64) int64 {
	return stkCreate(c, "/api/products", map[string]any{"category_id": cat, "name": "Микрофибра", "unit": "pcs",
		"package_size": 1, "wear_limit": limit})
}

// ragCard — карточка позиции (GET /api/products/{id}).
func ragCard(t *testing.T, c *client, id int64) catalog.ProductCard {
	t.Helper()
	return decode[struct {
		Product catalog.ProductCard `json:"product"`
	}](t, c.do(http.MethodGet, stkPath("/api/products/", id, ""), nil)).Product
}

func ragStart(t *testing.T, c *client, product int64, body any) ragStarted {
	t.Helper()
	rec := c.do(http.MethodPost, stkPath("/api/products/", product, "/rags"), body)
	expect(t, rec, http.StatusCreated, "в работу")
	return decode[ragStarted](t, rec)
}

func ragWash(t *testing.T, c *client, path string, ids ...int64) []stock.Rag {
	t.Helper()
	rec := c.do(http.MethodPost, path, map[string]any{"ids": ids})
	expect(t, rec, http.StatusOK, path)
	return decode[ragList](t, rec).Rags
}

func TestRagStartNumbersAndStock(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := ragProduct(c, cat, 10)
	stkBuy(c, p, 3, 10000, nil)

	s1 := ragStart(t, c, p, map[string]any{})
	if s1.Rag.Number != 1 || s1.Rag.Label != "№ 1" || s1.Rag.Status != "active" || s1.Rag.WearLimit == nil || *s1.Rag.WearLimit != 10 ||
		s1.Rag.Level != "ok" || s1.Rag.Washes != 0 || s1.Rag.RetireReason != nil || s1.Rag.LastWashedAt != nil ||
		len(s1.MovementIDs) != 1 || s1.Product.Stock.Total != 2 {
		t.Fatalf("первая: %+v", s1)
	}
	if s1.Product.Rags == nil || s1.Product.Rags.Active != 1 {
		t.Fatalf("сводка: %+v", s1.Product.Rags)
	}
	// Журнал: ровно один обычный расход 1 шт.
	ms := decode[struct {
		Movements []catalog.Movement `json:"movements"`
	}](t, c.do(http.MethodGet, stkPath("/api/movements?product_id=", p, ""), nil)).Movements
	if ms[0].ID != s1.MovementIDs[0] || ms[0].Type != "consume" || ms[0].QtyDelta != -1 {
		t.Fatalf("движение: %+v", ms[0])
	}
	// Своя метка; номер идёт дальше даже после списания.
	s2 := ragStart(t, c, p, map[string]any{"label": " Синяя "})
	if s2.Rag.Number != 2 || s2.Rag.Label != "Синяя" {
		t.Fatalf("вторая: %+v", s2.Rag)
	}
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", s2.Rag.ID, "/retire"), map[string]any{"reason": "lost"}), http.StatusOK, "списание")
	s3 := ragStart(t, c, p, map[string]any{})
	if s3.Rag.Number != 3 || s3.Product.Stock.Total != 0 {
		t.Fatalf("третья: %+v", s3)
	}
	// Склад пуст: нехватка в формате расхода, тряпка не появилась.
	rec := c.do(http.MethodPost, stkPath("/api/products/", p, "/rags"), map[string]any{})
	expect(t, rec, http.StatusConflict, "нет в запасе")
	if sh := decode[stkShortageBody](t, rec); sh.Available != 0 || !strings.Contains(sh.Error, "Не хватает") {
		t.Fatalf("нехватка: %+v", sh)
	}
	all := decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags
	act := decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags"), nil)).Rags
	if len(all) != 3 || len(act) != 2 || act[0].Number != 1 || act[1].Number != 3 {
		t.Fatalf("списки: all=%d active=%+v", len(all), act)
	}
	if all[1].Status != "retired" || all[1].RetireReason == nil || *all[1].RetireReason != "lost" || all[1].RetiredAt == nil {
		t.Fatalf("списанная: %+v", all[1])
	}
	// Метка: переименование и границы.
	rec = c.do(http.MethodPatch, stkPath("/api/rags/", s1.Rag.ID, ""), map[string]any{"label": "Красная"})
	expect(t, rec, http.StatusOK, "метка")
	if decode[stock.Rag](t, rec).Label != "Красная" {
		t.Fatal("метка не изменилась")
	}
	for _, l := range []string{"", "  ", strings.Repeat("я", 21)} {
		catExpectField(t, c, http.MethodPatch, stkPath("/api/rags/", s1.Rag.ID, ""), map[string]any{"label": l}, 400, "label")
	}
	catExpectField(t, c, http.MethodPost, stkPath("/api/products/", p, "/rags"), map[string]any{"label": strings.Repeat("я", 21)}, 400, "label")
}

func TestRagStartRequiresLimitAndIdempotent(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")

	plain := stkProduct(c, cat, "Салфетки", "pcs", 1)
	stkBuy(c, plain, 1, 100, nil)
	rec := c.do(http.MethodPost, stkPath("/api/products/", plain, "/rags"), map[string]any{})
	expect(t, rec, http.StatusConflict, "нет ресурса")
	if b := decode[errBody](t, rec); b.Field != "wear_limit" || !strings.Contains(b.Error, "Сначала задайте ресурс стирок") {
		t.Fatalf("нет ресурса: %+v", b)
	}
	if sum := ragCard(t, c, plain); sum.WearLimit != nil || sum.Rags != nil {
		t.Fatalf("без ресурса: %+v", sum)
	}
	// Ничего не списалось.
	if got := decode[struct {
		Product catalog.ProductCard `json:"product"`
	}](t, c.do(http.MethodGet, stkPath("/api/products/", plain, ""), nil)).Product.Stock.Total; got != 1 {
		t.Fatalf("остаток %d", got)
	}

	p := ragProduct(c, cat, 5)
	stkBuy(c, p, 2, 100, nil)
	body := map[string]any{"client_op_id": "rag-op-0001"}
	first := ragStart(t, c, p, body)
	rec = c.do(http.MethodPost, stkPath("/api/products/", p, "/rags"), body)
	expect(t, rec, http.StatusOK, "повтор")
	if rec.Header().Get("X-Garage-Replayed") != "1" {
		t.Fatal("нет X-Garage-Replayed")
	}
	again := decode[ragStarted](t, rec)
	if again.Rag.ID != first.Rag.ID || again.Product.Stock.Total != 1 {
		t.Fatalf("повтор: %+v", again)
	}
	if n := len(decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags); n != 1 {
		t.Fatalf("тряпок %d", n)
	}
	catExpectField(t, c, http.MethodPost, stkPath("/api/products/", p, "/rags"), map[string]any{"client_op_id": "!"}, 400, "client_op_id")
}

func TestRagWashLevelsAndAtomicity(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := ragProduct(c, cat, 10)
	stkBuy(c, p, 3, 100, nil)
	a, b, d := ragStart(t, c, p, map[string]any{}).Rag, ragStart(t, c, p, map[string]any{}).Rag, ragStart(t, c, p, map[string]any{}).Rag

	// Уровни: 8 из 10 — ok, 9 — soon (90 %), 10 — due.
	for i := 0; i < 8; i++ {
		ragWash(t, c, "/api/rags/wash", a.ID)
	}
	if r := ragWash(t, c, "/api/rags/wash", a.ID, a.ID); len(r) != 1 || r[0].Washes != 9 || r[0].Level != "soon" || r[0].LastWashedAt == nil {
		t.Fatalf("90 %%: %+v", r)
	}
	if r := ragWash(t, c, "/api/rags/wash", a.ID, b.ID); r[0].Washes != 10 || r[0].Level != "due" || r[1].Washes != 1 || r[1].Level != "ok" {
		t.Fatalf("100 %%: %+v", r)
	}
	sum := ragCard(t, c, p)
	if sum.Rags == nil || *sum.Rags != (catalog.RagCounts{Active: 3, Soon: 0, Due: 1}) || sum.WearLimit == nil {
		t.Fatalf("сводка: %+v", sum.Rags)
	}
	ragWash(t, c, "/api/rags/wash", b.ID, b.ID)
	for i := 0; i < 8; i++ {
		ragWash(t, c, "/api/rags/wash", b.ID)
	}
	ragWash(t, c, "/api/rags/wash", d.ID)
	list := decode[struct {
		Products []catalog.ProductSummary `json:"products"`
	}](t, c.do(http.MethodGet, "/api/products", nil)).Products
	if *list[0].Rags != (catalog.RagCounts{Active: 3, Soon: 0, Due: 2}) {
		t.Fatalf("список: %+v", list[0].Rags)
	}

	// Всё или ничего: чужой/несуществующий/списанный id — 404, стирок не прибавилось.
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", d.ID, "/retire"), map[string]any{"reason": "worn"}), http.StatusOK, "списание")
	before := decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags
	for _, ids := range [][]int64{{a.ID, 99999}, {a.ID, d.ID}} {
		expect(t, c.do(http.MethodPost, "/api/rags/wash", map[string]any{"ids": ids}), http.StatusNotFound, "стирка с лишним id")
		expect(t, c.do(http.MethodPost, "/api/rags/unwash", map[string]any{"ids": ids}), http.StatusNotFound, "отмена с лишним id")
	}
	after := decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags
	for i := range before {
		if before[i] != after[i] && (before[i].Washes != after[i].Washes) {
			t.Fatalf("изменилось: %+v -> %+v", before[i], after[i])
		}
	}
	// Повторное списание — 404; причина проверяется.
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", d.ID, "/retire"), map[string]any{"reason": "worn"}), http.StatusNotFound, "повтор списания")
	catExpectField(t, c, http.MethodPost, stkPath("/api/rags/", a.ID, "/retire"), map[string]any{"reason": "сломалась"}, 400, "reason")
	// Пределы списка.
	catExpectField(t, c, http.MethodPost, "/api/rags/wash", map[string]any{"ids": []int64{}}, 400, "ids")
	many := make([]int64, 101)
	for i := range many {
		many[i] = int64(i + 1)
	}
	catExpectField(t, c, http.MethodPost, "/api/rags/wash", map[string]any{"ids": many}, 400, "ids")

	// Отмена: −1, не ниже 0, уровень пересчитывается.
	if r := ragWash(t, c, "/api/rags/unwash", a.ID); r[0].Washes != 9 || r[0].Level != "soon" {
		t.Fatalf("отмена: %+v", r)
	}
	// Нижняя граница: у b 10 стирок, снимаем с запасом — не уйдёт ниже нуля.
	for i := 0; i < 12; i++ {
		ragWash(t, c, "/api/rags/unwash", b.ID)
	}
	if r := ragWash(t, c, "/api/rags/unwash", b.ID); r[0].Washes != 0 || r[0].Level != "ok" || r[0].LastWashedAt != nil {
		t.Fatalf("ниже нуля: %+v", r)
	}
}

func TestRagWearLimitOnProducts(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	ml := stkProduct(c, cat, "Шампунь", "ml", 500)
	pcs := stkProduct(c, cat, "Губка", "pcs", 1)

	// ml с ресурсом — 400 wear_limit (создание и правка), диапазон 1…1000.
	catExpectField(t, c, http.MethodPost, "/api/products", map[string]any{"category_id": cat, "name": "М", "unit": "ml",
		"package_size": 1, "wear_limit": 5}, 400, "wear_limit")
	catExpectField(t, c, http.MethodPatch, stkPath("/api/products/", ml, ""), map[string]any{"wear_limit": 5}, 400, "wear_limit")
	for _, v := range []any{0, 1001, -3, "x", 1.5} {
		catExpectField(t, c, http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"wear_limit": v}, 400, "wear_limit")
	}
	for _, v := range []int{1, 1000} {
		rec := c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"wear_limit": v})
		expect(t, rec, http.StatusOK, "граница")
		if got := decode[catalog.ProductCard](t, rec); got.WearLimit == nil || *got.WearLimit != int64(v) || got.Rags == nil {
			t.Fatalf("после правки: %+v", got)
		}
	}
	// Без поля — не меняется; null — снимает; у ml всегда null.
	if got := decode[catalog.ProductCard](t, c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"note": "x"})); got.WearLimit == nil {
		t.Fatal("ресурс слетел")
	}
	got := decode[catalog.ProductCard](t, c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"wear_limit": nil}))
	if got.WearLimit != nil || got.Rags != nil {
		t.Fatalf("снятие: %+v", got)
	}
	if body := c.do(http.MethodGet, stkPath("/api/products/", ml, ""), nil).Body.String(); !strings.Contains(body, `"wear_limit":null`) || !strings.Contains(body, `"rags":null`) {
		t.Fatalf("ml: %s", body)
	}
	// Смена единицы на мл снимает ресурс.
	expect(t, c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"wear_limit": 7}), http.StatusOK, "вернули")
	got = decode[catalog.ProductCard](t, c.do(http.MethodPatch, stkPath("/api/products/", pcs, ""), map[string]any{"unit": "ml"}))
	if got.WearLimit != nil {
		t.Fatalf("смена единицы: %+v", got)
	}
	// Позиции без позиции-владельца: тряпки чужой/несуществующей позиции — 404.
	expect(t, c.do(http.MethodGet, "/api/products/99999/rags", nil), http.StatusNotFound, "нет позиции")
	expect(t, c.do(http.MethodPost, "/api/products/99999/rags", map[string]any{}), http.StatusNotFound, "в работу: нет позиции")
}

func TestRagUnstart(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := ragProduct(c, cat, 10)
	stkBuy(c, p, 4, 100, nil)
	r1 := ragStart(t, c, p, map[string]any{})
	r2 := ragStart(t, c, p, map[string]any{})

	// После стирки и после списания — 409, ничего не меняется.
	ragWash(t, c, "/api/rags/wash", r1.Rag.ID)
	rec := c.do(http.MethodPost, stkPath("/api/rags/", r1.Rag.ID, "/unstart"), map[string]any{})
	expect(t, rec, http.StatusConflict, "после стирки")
	if !strings.Contains(decode[errBody](t, rec).Error, "отменить нельзя") {
		t.Fatalf("текст: %s", rec.Body.String())
	}
	ragWash(t, c, "/api/rags/unwash", r1.Rag.ID)
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", r1.Rag.ID, "/unstart"), map[string]any{}), http.StatusOK, "после отмены стирки")
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", r1.Rag.ID, "/unstart"), map[string]any{}), http.StatusNotFound, "повтор")
	expect(t, c.do(http.MethodPost, "/api/rags/99999/unstart", map[string]any{}), http.StatusNotFound, "нет тряпки")

	// Успех: запас вернулся, тряпки нет, номер снова свободен.
	rec = c.do(http.MethodPost, stkPath("/api/rags/", r2.Rag.ID, "/unstart"), map[string]any{})
	expect(t, rec, http.StatusOK, "отмена")
	got := decode[struct {
		Product catalog.ProductSummary `json:"product"`
	}](t, rec).Product
	if got.ID != p || got.Stock.Total != 4 || got.Rags == nil || got.Rags.Active != 0 {
		t.Fatalf("после отмены: %+v %+v", got.Stock, got.Rags)
	}
	if n := len(decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags); n != 0 {
		t.Fatalf("тряпок %d", n)
	}
	if s := ragStart(t, c, p, map[string]any{}); s.Rag.Number != 1 {
		t.Fatalf("номер после отмены: %d", s.Rag.Number)
	}

	// Списанная — 409; расход уже отменён вручную — 409.
	s2 := ragStart(t, c, p, map[string]any{})
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", s2.Rag.ID, "/retire"), map[string]any{"reason": "lost"}), http.StatusOK, "списание")
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", s2.Rag.ID, "/unstart"), map[string]any{}), http.StatusConflict, "списанная")
	_, err := e.db.Exec(`DELETE FROM movements WHERE id = ?`, s2.MovementIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	s3 := ragStart(t, c, p, map[string]any{})
	_, _ = e.db.Exec(`DELETE FROM movements WHERE id = ?`, s3.MovementIDs[0])
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", s3.Rag.ID, "/unstart"), map[string]any{}), http.StatusConflict, "расхода нет")
}

// Ручная отмена расхода тряпки (revert): id движений переиспользуются, поэтому
// связь тряпки с движением не должна повисать.
func TestRagManualRevertDetaches(t *testing.T) {
	e := newEnv(t, false)
	c := stkSignIn(e, "owner")
	cat := stkCategory(c, "Тест")
	p := ragProduct(c, cat, 10)
	stkBuy(c, p, 3, 100, nil)
	total := func() int64 { return ragCard(t, c, p).Stock.Total }

	// Без стирок: тряпка исчезает, шт вернулась.
	a := ragStart(t, c, p, map[string]any{})
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": a.MovementIDs}), http.StatusOK, "revert без стирок")
	if n := len(decode[ragList](t, c.do(http.MethodGet, stkPath("/api/products/", p, "/rags?all=1"), nil)).Rags); n != 0 || total() != 3 {
		t.Fatalf("тряпок %d, запас %d", n, total())
	}

	// Со стирками: тряпка остаётся, связь обнулена, unstart — 409.
	b := ragStart(t, c, p, map[string]any{})
	ragWash(t, c, "/api/rags/wash", b.Rag.ID)
	expect(t, c.do(http.MethodPost, "/api/movements/revert", map[string]any{"ids": b.MovementIDs}), http.StatusOK, "revert со стирками")
	var link *int64
	if err := e.db.QueryRow(`SELECT consume_movement_id FROM rags WHERE id = ?`, b.Rag.ID).Scan(&link); err != nil || link != nil {
		t.Fatalf("связь: %v %v", link, err)
	}
	// Новый расход получает переиспользованный id — unstart старой тряпки его не трогает.
	other := decode[stkConsumeBody](t, c.do(http.MethodPost, "/api/consume", map[string]any{"product_id": p, "qty": 1}))
	if other.Movements[0].ID != b.MovementIDs[0] {
		t.Logf("id не переиспользован (%d и %d): проверка слабее", other.Movements[0].ID, b.MovementIDs[0])
	}
	before := total()
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", b.Rag.ID, "/unstart"), map[string]any{}), http.StatusConflict, "unstart после revert")
	if total() != before {
		t.Fatal("unstart тронул чужой расход")
	}
	var n int
	_ = e.db.QueryRow(`SELECT COUNT(*) FROM movements WHERE id = ?`, other.Movements[0].ID).Scan(&n)
	if n != 1 {
		t.Fatal("новый расход удалён")
	}

	// Экспорт после ручной отмены отдаёт null и импортируется.
	file := trExport(t, c)
	for _, r := range file.Rags {
		if r.ID == b.Rag.ID && r.ConsumeMovementID != nil {
			t.Fatalf("экспорт: висячая связь %d", *r.ConsumeMovementID)
		}
	}
	u := stkSignIn(e, "friend")
	expect(t, u.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт после revert")

	// Старый файл с висячим id: связь обнуляется, файл не отвергается.
	for i := range file.Rags {
		bad := int64(98765)
		file.Rags[i].ConsumeMovementID = &bad
	}
	u2 := stkSignIn(e, "friend2")
	expect(t, u2.do(http.MethodPost, "/api/import", file), http.StatusOK, "импорт с висячим id")
	for _, r := range trExport(t, u2).Rags {
		if r.ConsumeMovementID != nil {
			t.Fatalf("связь не обнулена: %d", *r.ConsumeMovementID)
		}
	}

	// Висячая ссылка из БД (как в старых данных): unstart проверяет движение.
	d := ragStart(t, c, p, map[string]any{})
	if _, err := e.db.Exec(`UPDATE rags SET consume_movement_id = ? WHERE id = ?`, other.Movements[0].ID, d.Rag.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE movements SET qty_delta = -2 WHERE id = ?`, other.Movements[0].ID); err != nil {
		t.Fatal(err)
	}
	expect(t, c.do(http.MethodPost, stkPath("/api/rags/", d.Rag.ID, "/unstart"), map[string]any{}), http.StatusConflict, "чужое движение")
}
