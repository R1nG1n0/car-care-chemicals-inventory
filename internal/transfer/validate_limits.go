package transfer

import "fmt"

// Пределы числа записей в файле: 100 МБ JSON иначе вместили бы миллионы
// мелких объектов, а проверка и вставка держат их в памяти и в единственном
// соединении с базой. Больше любого реального личного склада с запасом.
const (
	maxMovements     = 200000
	maxUnits         = 50000
	maxPurchases     = 20000
	maxPurchaseLines = 60000
	maxProducts      = 10000
	maxOtherRecords  = 10000 // категории, места, магазины, шаблоны, применения шаблонов, список покупок

	// maxSort — предел порядка сортировки справочников, как в обычном API.
	maxSort = 1000000000
)

// checkLimits отклоняет файл с непомерным числом записей до любой другой проверки.
func checkLimits(f *File) error {
	lines := 0
	for _, p := range f.Purchases {
		lines += len(p.Lines)
	}
	for _, l := range []struct {
		name string
		n    int
		max  int
	}{
		{"movements", len(f.Movements), maxMovements},
		{"units", len(f.Units), maxUnits},
		{"products", len(f.Products), maxProducts},
		{"purchases", len(f.Purchases), maxPurchases},
		{"purchases.lines", lines, maxPurchaseLines},
		{"categories", len(f.Categories), maxOtherRecords},
		{"locations", len(f.Locations), maxOtherRecords},
		{"stores", len(f.Stores), maxOtherRecords},
		{"purposes", len(f.Purposes), maxOtherRecords},
		{"recipes", len(f.Recipes), maxOtherRecords},
		{"recipe_runs", len(f.RecipeRuns), maxOtherRecords},
		{"shopping", len(f.Shopping), maxOtherRecords},
	} {
		if l.n > l.max {
			return bad(l.name, fmt.Sprintf("Слишком много записей: %d, допустимо не больше %d", l.n, l.max))
		}
	}
	return nil
}

// checkUnitState сверяет статус экземпляра с остатком и отметками времени
// так, как их ведёт обычный API (internal/stock): sealed — нетронутый, open —
// частично израсходованный, finished и written_off — пустой и закрытый.
func checkUnitState(path func(string) string, u *Unit) error {
	switch u.Status {
	case "sealed":
		if u.RemainingQty != u.InitialQty {
			return bad(path("remaining_qty"), "У запечатанного экземпляра остаток равен объёму")
		}
		if u.OpenedAt != nil {
			return bad(path("opened_at"), "Запечатанный экземпляр не бывает вскрытым")
		}
	case "open":
		if u.RemainingQty < 1 {
			return bad(path("remaining_qty"), "У вскрытого экземпляра остаток больше нуля")
		}
	default: // finished, written_off
		if u.RemainingQty != 0 {
			return bad(path("remaining_qty"), "У законченного или списанного экземпляра остаток равен нулю")
		}
		if u.FinishedAt == nil {
			return bad(path("finished_at"), "У законченного или списанного экземпляра нужна дата окончания")
		}
	}
	return nil
}

// checkBalances: остаток каждого экземпляра равен сумме изменений его
// движений. Первое движение (purchase или adjust «Начальный остаток») уже
// содержит стартовый остаток, поэтому начальный объём к сумме не добавляется.
// Экземпляр без движений допустим только пустой.
func checkBalances(f *File) error {
	sum := make(map[int64]int64, len(f.Units))
	for _, m := range f.Movements {
		sum[m.UnitID] += m.QtyDelta
	}
	for i := range f.Units {
		u := &f.Units[i]
		if got := sum[u.ID]; got != u.RemainingQty {
			return bad(at("units", i, "remaining_qty"),
				fmt.Sprintf("Остаток %d не сходится с суммой движений %d", u.RemainingQty, got))
		}
	}
	return nil
}
