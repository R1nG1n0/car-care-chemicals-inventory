package stock

import (
	"database/sql"
	"testing"
	"time"
)

func idemRender(_ *sql.Tx, res ConsumeResult) ([]byte, error) {
	return []byte(`{"consumed":` + string(rune('0'+res.Consumed%10)) + `}`), nil
}

func i64(v int64) *int64 { return &v }

// Повтор с тем же номером не списывает второй раз и возвращает прежний ответ.
func TestConsumeOpReplay(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	unit := e.stkBuy(uid, p, "2026-10-01", 1, 50000, nil)[0]
	in := ConsumeInput{ProductID: p, Qty: 5, Op: Op{ClientOpID: "op-12345678"}}

	res, body, replay, err := e.svc.ConsumeOp(e.ctx, uid, in, idemRender)
	if err != nil || replay || res.Consumed != 5 || string(body) != `{"consumed":5}` {
		t.Fatalf("первый вызов: %+v %q %v %v", res, body, replay, err)
	}
	res, body2, replay, err := e.svc.ConsumeOp(e.ctx, uid, in, idemRender)
	if err != nil || !replay || string(body2) != string(body) || res.Consumed != 0 {
		t.Fatalf("повтор: %+v %q %v %v", res, body2, replay, err)
	}
	stkWant(t, e.stkUnit(unit), 495, "open")
	if n := e.stkCount(`SELECT COUNT(*) FROM movements WHERE type = 'consume'`); n != 1 {
		t.Fatalf("движений расхода %d", n)
	}

	// Тот же номер у другого вида операции — конфликт.
	rid, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "М", Items: []ItemInput{{ProductID: p, Qty: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, err = e.svc.ApplyRecipeOp(e.ctx, uid, rid, []ItemInput{{ProductID: p, Qty: 1}}, in.Op, nil)
	if ce := stkIs[*ConflictError](t, err, "другой вид"); ce.Field != "client_op_id" {
		t.Fatalf("поле %q", ce.Field)
	}
	stkWant(t, e.stkUnit(unit), 495, "open")

	// Другой пользователь с тем же номером списывает у себя.
	uid2, cat2 := e.stkUser("b")
	p2 := e.stkProduct(uid2, cat2, "Gsf", "ml", 500)
	e.stkBuy(uid2, p2, "2026-10-01", 1, 50000, nil)
	_, _, replay, err = e.svc.ConsumeOp(e.ctx, uid2, ConsumeInput{ProductID: p2, Qty: 7, Op: in.Op}, idemRender)
	if err != nil || replay {
		t.Fatalf("другой пользователь: %v %v", replay, err)
	}
}

// Нехватка и прочие ошибки номер не сохраняют: повтор после пополнения проходит.
func TestConsumeOpErrorNotStored(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	in := ConsumeInput{ProductID: p, Qty: 100, Op: Op{ClientOpID: "op-short-1"}}
	_, _, _, err := e.svc.ConsumeOp(e.ctx, uid, in, idemRender)
	stkIs[*ShortageError](t, err, "нехватка")
	if n := e.stkCount(`SELECT COUNT(*) FROM client_ops`); n != 0 {
		t.Fatalf("номер сохранён при ошибке: %d", n)
	}
	e.stkBuy(uid, p, "2026-10-01", 1, 50000, nil)
	if _, _, replay, err := e.svc.ConsumeOp(e.ctx, uid, in, idemRender); err != nil || replay {
		t.Fatalf("после пополнения: %v %v", replay, err)
	}
}

func TestOpValidation(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	e.stkBuy(uid, p, "2026-10-01", 1, 50000, nil)
	for _, id := range []string{"short", "has space 123", "кириллица-12345", string(make([]byte, 65))} {
		_, _, _, err := e.svc.ConsumeOp(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1, Op: Op{ClientOpID: id}}, nil)
		if ve := stkIs[*ValidationError](t, err, "номер "+id); ve.Field != "client_op_id" {
			t.Fatalf("поле %q", ve.Field)
		}
	}
	// UUID подходит.
	if _, _, _, err := e.svc.ConsumeOp(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1,
		Op: Op{ClientOpID: "123e4567-e89b-12d3-a456-426614174000"}}, nil); err != nil {
		t.Fatal(err)
	}
}

// ФТ-12.4.2: время с телефона в пределах — в движениях и экземпляре, иначе now.
func TestOccurredAt(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 100)
	e.stkBuy(uid, p, "2026-10-01", 20, 1000, nil)
	now := e.clock.Now()
	cases := []struct {
		name string
		at   *int64
		want int64
	}{
		{"нет", nil, now.Unix()},
		{"час назад", i64(now.Add(-time.Hour).Unix()), now.Add(-time.Hour).Unix()},
		{"29 дней назад", i64(now.Add(-29 * 24 * time.Hour).Unix()), now.Add(-29 * 24 * time.Hour).Unix()},
		{"+4 минуты", i64(now.Add(4 * time.Minute).Unix()), now.Add(4 * time.Minute).Unix()},
		{"+6 минут", i64(now.Add(6 * time.Minute).Unix()), now.Unix()},
		{"31 день назад", i64(now.Add(-31 * 24 * time.Hour).Unix()), now.Unix()},
	}
	for _, c := range cases {
		res, _, _, err := e.svc.ConsumeOp(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1, Op: Op{OccurredAt: c.at}}, nil)
		if err != nil {
			t.Fatal(c.name, err)
		}
		if got := e.stkCount(`SELECT created_at FROM movements WHERE id = ?`, res.MovementIDs[0]); got != c.want {
			t.Fatalf("%s: время движения %d, ожидалось %d", c.name, got, c.want)
		}
	}
	// Шаблон: движения, запись применения и закончившийся экземпляр — тем же временем.
	q := e.stkProduct(uid, cat, "Одна", "ml", 10)
	u := e.stkBuy(uid, q, "2026-10-01", 1, 1000, nil)[0]
	rid, err := e.svc.CreateRecipe(e.ctx, uid, RecipeInput{Name: "М", Items: []ItemInput{{ProductID: q, Qty: 10}}})
	if err != nil {
		t.Fatal(err)
	}
	at := now.Add(-2 * time.Hour).Unix()
	res, _, _, err := e.svc.ApplyRecipeOp(e.ctx, uid, rid, []ItemInput{{ProductID: q, Qty: 10}}, Op{OccurredAt: &at}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if e.stkCount(`SELECT created_at FROM movements WHERE id = ?`, res.MovementIDs[0]) != at ||
		e.stkCount(`SELECT created_at FROM recipe_runs WHERE id = ?`, res.RunID) != at ||
		e.stkCount(`SELECT opened_at FROM units WHERE id = ?`, u) != at ||
		e.stkCount(`SELECT finished_at FROM units WHERE id = ?`, u) != at {
		t.Fatal("время применения шаблона не из occurred_at")
	}
}

// Номера старше 90 дней удаляются при следующей записи.
func TestClientOpsCleanup(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Gsf", "ml", 500)
	e.stkBuy(uid, p, "2026-10-01", 1, 50000, nil)
	now := e.clock.Now().Unix()
	e.exec(`INSERT INTO client_ops (user_id, op_id, kind, response, created_at) VALUES (?, 'old-op-12345', 'consume', '{}', ?)`,
		uid, now-91*24*3600)
	e.exec(`INSERT INTO client_ops (user_id, op_id, kind, response, created_at) VALUES (?, 'fresh-op-1234', 'consume', '{}', ?)`,
		uid, now-89*24*3600)
	if _, _, _, err := e.svc.ConsumeOp(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 1, Op: Op{ClientOpID: "new-op-12345"}}, idemRender); err != nil {
		t.Fatal(err)
	}
	if e.stkCount(`SELECT COUNT(*) FROM client_ops WHERE op_id = 'old-op-12345'`) != 0 ||
		e.stkCount(`SELECT COUNT(*) FROM client_ops`) != 2 {
		t.Fatal("очистка старых номеров")
	}
}
