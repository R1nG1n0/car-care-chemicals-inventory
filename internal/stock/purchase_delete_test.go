package stock

import "testing"

func TestDeletePurchase(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Шампунь", "ml", 500)
	item := e.exec(`INSERT INTO shopping_items (user_id, product_id, created_at) VALUES (?, ?, 1)`, uid, p)

	id, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-02",
		Lines: []PurchaseLineInput{{ProductID: p, Packages: 2, PricePerPackage: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeletePurchase(e.ctx, uid, id); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"purchases", "purchase_lines", "units", "movements"} {
		if n := e.stkCount(`SELECT COUNT(*) FROM ` + table); n != 0 {
			t.Fatalf("в %s осталось %d строк", table, n)
		}
	}
	// Пункт, вычеркнутый покупкой, возвращается в список.
	if e.stkCount(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at IS NULL`, item) != 1 {
		t.Fatal("пункт списка покупок должен вернуться")
	}
	// Пункт, вычеркнутый вручную в другое время, не трогаем.
	other := e.exec(`INSERT INTO shopping_items (user_id, product_id, done_at, created_at) VALUES (?, ?, 5, 1)`, uid, p)
	id2, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-02",
		Lines: []PurchaseLineInput{{ProductID: p, Packages: 1, PricePerPackage: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeletePurchase(e.ctx, uid, id2); err != nil {
		t.Fatal(err)
	}
	if e.stkCount(`SELECT COUNT(*) FROM shopping_items WHERE id = ? AND done_at = 5`, other) != 1 {
		t.Fatal("чужая по времени отметка должна остаться")
	}
	// Повторное удаление — «не найдено».
	if err := e.svc.DeletePurchase(e.ctx, uid, id); !IsNotFound(err) {
		t.Fatalf("повтор: %v", err)
	}
}

func TestDeletePurchaseUsedUnits(t *testing.T) {
	e := stkSetup(t)
	uid, cat := e.stkUser("a")
	p := e.stkProduct(uid, cat, "Шампунь", "ml", 500)
	loc2 := e.stkLocation(uid, "Полка")

	cases := map[string]func(unit int64){
		"расход": func(int64) {
			if _, err := e.svc.Consume(e.ctx, uid, ConsumeInput{ProductID: p, Qty: 10}); err != nil {
				t.Fatal(err)
			}
		},
		"перемещение": func(u int64) {
			if err := e.svc.Move(e.ctx, uid, u, loc2); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, touch := range cases {
		t.Run(name, func(t *testing.T) {
			id, err := e.svc.CreatePurchase(e.ctx, uid, PurchaseInput{Date: "2026-10-02",
				Lines: []PurchaseLineInput{{ProductID: p, Packages: 1, PricePerPackage: 1000}}})
			if err != nil {
				t.Fatal(err)
			}
			pur, _ := e.svc.Purchase(e.ctx, uid, id)
			touch(pur.Lines[0].UnitIDs[0])
			err = e.svc.DeletePurchase(e.ctx, uid, id)
			ce := stkIs[*ConflictError](t, err, "удаление использованной покупки")
			if ce.Message != "Экземпляры покупки уже использованы — отменить нельзя" {
				t.Fatalf("сообщение: %q", ce.Message)
			}
			if _, err := e.svc.Purchase(e.ctx, uid, id); err != nil {
				t.Fatalf("покупка должна остаться: %v", err)
			}
		})
	}
}

func TestDeletePurchaseForeignUser(t *testing.T) {
	e := stkSetup(t)
	a, catA := e.stkUser("a")
	b, _ := e.stkUser("b")
	p := e.stkProduct(a, catA, "Шампунь", "ml", 500)
	id, err := e.svc.CreatePurchase(e.ctx, a, PurchaseInput{Date: "2026-10-02",
		Lines: []PurchaseLineInput{{ProductID: p, Packages: 1, PricePerPackage: 1000}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.DeletePurchase(e.ctx, b, id); !IsNotFound(err) {
		t.Fatalf("чужая покупка: %v", err)
	}
	if _, err := e.svc.Purchase(e.ctx, a, id); err != nil {
		t.Fatalf("покупка владельца должна остаться: %v", err)
	}
}
