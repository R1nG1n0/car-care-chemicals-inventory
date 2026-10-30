package transfer

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CSV для Excel в русской локали (ФТ-10.3.2): UTF-8 с BOM, разделитель «;»,
// строки через CRLF, даты ДД.ММ.ГГГГ, деньги в рублях с запятой.

const csvBOM = "\xEF\xBB\xBF"

// csvText защищает от CSV-инъекции: Excel выполняет ячейку, начинающуюся
// с = + - @ табуляции или возврата каретки, как формулу. Такой текст
// получает апостроф в начале. Только для текстов пользователя — числа,
// которые формирует сервер (в том числе отрицательные), не трогаются.
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// rubles — копейки в рубли с запятой: 119000 → «1190,00».
func rubles(kopecks int64) string {
	sign := ""
	if kopecks < 0 {
		sign, kopecks = "-", -kopecks
	}
	return fmt.Sprintf("%s%d,%02d", sign, kopecks/100, kopecks%100)
}

// ruDate — «2026-10-02» → «02.10.2026».
func ruDate(iso string) string {
	if len(iso) != 10 || iso[4] != '-' || iso[7] != '-' {
		return iso
	}
	return iso[8:] + "." + iso[5:7] + "." + iso[:4]
}

// ruDateTime — секунды Unix → «02.10.2026 14:05» в поясе клиента (tz — минуты от UTC).
func ruDateTime(sec int64, tz int) string {
	return time.Unix(sec, 0).UTC().Add(time.Duration(tz) * time.Minute).Format("02.01.2006 15:04")
}

func unitName(unit string) string {
	if unit == "pcs" {
		return "шт"
	}
	return "мл"
}

func movementType(t string) string {
	switch t {
	case "purchase":
		return "Покупка"
	case "consume":
		return "Расход"
	case "writeoff":
		return "Списание"
	case "move":
		return "Перемещение"
	case "adjust":
		return "Корректировка"
	}
	return t
}

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func nullText(v sql.NullString) string { return csvText(v.String) }

// csvTable собирает таблицу в памяти: ошибка запроса не оставит полуфайл.
type csvTable struct {
	buf bytes.Buffer
	w   *csv.Writer
}

func newCSV(header ...string) *csvTable {
	t := &csvTable{}
	t.buf.WriteString(csvBOM)
	t.w = csv.NewWriter(&t.buf)
	t.w.Comma = ';'
	t.w.UseCRLF = true
	_ = t.w.Write(header)
	return t
}

func (t *csvTable) row(cells ...string) { _ = t.w.Write(cells) }

func (t *csvTable) bytes() ([]byte, error) {
	t.w.Flush()
	if err := t.w.Error(); err != nil {
		return nil, err
	}
	return t.buf.Bytes(), nil
}

// ProductsCSV — позиции с остатками.
func (s *Service) ProductsCSV(ctx context.Context, userID int64) ([]byte, error) {
	places := map[int64][]string{}
	err := each(ctx, s.db, "места позиций", `
		SELECT DISTINCT u.product_id, l.name, l.sort FROM units u
		JOIN locations l ON l.user_id = u.user_id AND l.id = u.location_id
		WHERE u.user_id = ? AND u.status IN ('sealed', 'open') ORDER BY l.sort, l.name`,
		[]any{userID}, func(r *sql.Rows) error {
			var id, sort int64
			var name string
			if err := r.Scan(&id, &name, &sort); err != nil {
				return err
			}
			places[id] = append(places[id], name)
			return nil
		})
	if err != nil {
		return nil, err
	}
	purposes := map[int64][]string{}
	err = each(ctx, s.db, "назначения позиций", `
		SELECT pp.product_id, c.name FROM product_purposes pp
		JOIN purposes c ON c.user_id = pp.user_id AND c.id = pp.purpose_id
		WHERE pp.user_id = ? ORDER BY c.sort, c.name, c.id`,
		[]any{userID}, func(r *sql.Rows) error {
			var id int64
			var name string
			if err := r.Scan(&id, &name); err != nil {
				return err
			}
			purposes[id] = append(purposes[id], name)
			return nil
		})
	if err != nil {
		return nil, err
	}
	t := newCSV("Название", "Бренд", "Категория", "Единица", "Объём упаковки", "Остаток", "Экземпляров",
		"Минимальный остаток", "Ниже минимума", "Места", "Штрихкод", "Заметка", "В архиве",
		"Назначения", "pH", "Оценка", "Не брать больше")
	err = each(ctx, s.db, "позиции", `
		SELECT p.id, p.name, p.brand, c.name, p.unit, p.package_size,
		       COALESCE(SUM(u.remaining_qty), 0), COUNT(u.id), p.min_stock, p.barcode, p.note, p.archived,
		       p.ph_x10, p.rating, p.do_not_buy
		FROM products p
		JOIN categories c ON c.user_id = p.user_id AND c.id = p.category_id
		LEFT JOIN units u ON u.user_id = p.user_id AND u.product_id = p.id AND u.status IN ('sealed', 'open')
		WHERE p.user_id = ?
		GROUP BY p.id ORDER BY p.archived, c.sort, c.name, p.name, p.id`, []any{userID},
		func(r *sql.Rows) error {
			var id, size, total, count int64
			var name, brand, category, unit, barcode, note string
			var minStock, phX10, rating sql.NullInt64
			var archived, doNotBuy bool
			if err := r.Scan(&id, &name, &brand, &category, &unit, &size, &total, &count, &minStock,
				&barcode, &note, &archived, &phX10, &rating, &doNotBuy); err != nil {
				return err
			}
			min, low := "", "нет"
			if minStock.Valid {
				min = itoa(minStock.Int64)
				low = yesNo(total < minStock.Int64)
			}
			ph, stars := "", ""
			if phX10.Valid {
				ph = fmt.Sprintf("%d,%d", phX10.Int64/10, phX10.Int64%10)
			}
			if rating.Valid {
				stars = itoa(rating.Int64)
			}
			t.row(csvText(name), csvText(brand), csvText(category), unitName(unit), itoa(size), itoa(total),
				itoa(count), min, low, csvText(strings.Join(places[id], ", ")), csvText(barcode), csvText(note),
				yesNo(archived), csvText(strings.Join(purposes[id], ", ")), ph, stars, yesNo(doNotBuy))
			return nil
		})
	if err != nil {
		return nil, err
	}
	return t.bytes()
}

// PurchasesCSV — строки покупок: одна строка таблицы на строку покупки.
func (s *Service) PurchasesCSV(ctx context.Context, userID int64) ([]byte, error) {
	t := newCSV("Дата", "Магазин", "Позиция", "Бренд", "Упаковок", "Цена за упаковку, руб", "Сумма, руб", "Заметка")
	err := each(ctx, s.db, "покупки", `
		SELECT pu.date, st.name, p.name, p.brand, pl.packages, pl.price_per_package, pu.note
		FROM purchase_lines pl
		JOIN purchases pu ON pu.user_id = pl.user_id AND pu.id = pl.purchase_id
		JOIN products p ON p.user_id = pl.user_id AND p.id = pl.product_id
		LEFT JOIN stores st ON st.user_id = pu.user_id AND st.id = pu.store_id
		WHERE pl.user_id = ? ORDER BY pu.date, pu.id, pl.id`, []any{userID},
		func(r *sql.Rows) error {
			var date, product, brand, note string
			var store sql.NullString
			var packages, price int64
			if err := r.Scan(&date, &store, &product, &brand, &packages, &price, &note); err != nil {
				return err
			}
			t.row(ruDate(date), nullText(store), csvText(product), csvText(brand), itoa(packages),
				rubles(price), rubles(packages*price), csvText(note))
			return nil
		})
	if err != nil {
		return nil, err
	}
	return t.bytes()
}

// MovementsCSV — журнал движений; время — в часовом поясе клиента (tz, минуты).
func (s *Service) MovementsCSV(ctx context.Context, userID int64, tz int) ([]byte, error) {
	t := newCSV("Дата и время", "Тип", "Позиция", "Бренд", "Изменение", "Единица", "Откуда", "Куда", "Причина",
		"Стоимость, руб", "Раствор, мл", "Пропорция 1:N")
	err := each(ctx, s.db, "движения", `
		SELECT m.created_at, m.type, p.name, p.brand, p.unit, m.qty_delta, lf.name, lt.name, m.reason,
		       pl.price_per_package, u.initial_qty, m.solution_ml, m.solution_ratio
		FROM movements m
		JOIN units u ON u.user_id = m.user_id AND u.id = m.unit_id
		JOIN products p ON p.user_id = u.user_id AND p.id = u.product_id
		LEFT JOIN locations lf ON lf.user_id = m.user_id AND lf.id = m.from_location_id
		LEFT JOIN locations lt ON lt.user_id = m.user_id AND lt.id = m.to_location_id
		LEFT JOIN purchase_lines pl ON pl.user_id = u.user_id AND pl.id = u.purchase_line_id
		WHERE m.user_id = ? ORDER BY m.created_at, m.id`, []any{userID},
		func(r *sql.Rows) error {
			var at, delta int64
			var typ, product, brand, unit, reason string
			var from, to sql.NullString
			var price sql.NullInt64
			var initialQty, solML, solRatio sql.NullInt64
			if err := r.Scan(&at, &typ, &product, &brand, &unit, &delta, &from, &to, &reason, &price,
				&initialQty, &solML, &solRatio); err != nil {
				return err
			}
			cost := ""
			if price.Valid && initialQty.Valid && initialQty.Int64 > 0 && delta != 0 {
				abs := delta
				if abs < 0 {
					abs = -abs
				}
				cost = rubles((abs*price.Int64 + initialQty.Int64/2) / initialQty.Int64)
			}
			solution, ratio := "", ""
			if solML.Valid && solRatio.Valid {
				// Только N: «1:10» Excel и Numbers читают как время 01:10
				solution, ratio = itoa(solML.Int64), itoa(solRatio.Int64)
			}
			t.row(ruDateTime(at, tz), movementType(typ), csvText(product), csvText(brand), itoa(delta),
				unitName(unit), nullText(from), nullText(to), csvText(reason), cost, solution, ratio)
			return nil
		})
	if err != nil {
		return nil, err
	}
	return t.bytes()
}
