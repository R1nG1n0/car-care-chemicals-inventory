package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/barcode"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

// maxResults — сколько позиций показывается в ответе на поиск (ФТ-11.3.1).
const maxResults = 10

// searchText — поиск по названию и бренду (ФТ-11.3).
func (b *Bot) searchText(ctx context.Context, link Link, chatID int64, text string) {
	found, err := b.cat.Products(ctx, link.UserID, catalog.ProductFilter{Query: text})
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return
	}
	if len(found) == 0 {
		b.send(ctx, chatID, "Ничего не нашлось по запросу «"+telegram.Esc(label(text, 60))+"». "+
			"Проверьте написание или пришлите штрихкод.", nil)
		return
	}
	b.showFound(ctx, link, chatID, found)
}

// searchBarcode — поиск по штрихкоду: UPC-A и EAN-13 с нулём — один код (ФТ-11.4.1).
func (b *Bot) searchBarcode(ctx context.Context, link Link, chatID int64, code string) {
	list, err := b.cat.Products(ctx, link.UserID, catalog.ProductFilter{Query: barcode.SearchKey(code)})
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return
	}
	var found []catalog.ProductSummary
	for _, p := range list {
		if barcode.Same(p.Barcode, code) {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		b.send(ctx, chatID, "Штрихкод "+telegram.Esc(code)+" не найден. "+b.notFoundHint(), b.siteKeyboard())
		return
	}
	b.showFound(ctx, link, chatID, found)
}

func (b *Bot) siteKeyboard() *telegram.Keyboard {
	if strings.HasPrefix(b.siteURL, "https://") {
		return &telegram.Keyboard{Rows: [][]telegram.Button{telegram.Row(telegram.Link("Открыть сайт", b.siteURL))}}
	}
	return nil
}

// showFound: одна позиция — карточка, несколько — список до 10.
func (b *Bot) showFound(ctx context.Context, link Link, chatID int64, found []catalog.ProductSummary) {
	if len(found) == 1 {
		b.sendCard(ctx, link, chatID, found[0].ID)
		return
	}
	locs := b.locationNames(ctx, link.UserID)
	shown := found[:min(len(found), maxResults)]
	var sb strings.Builder
	sb.WriteString("Найдено: " + fmtCount(len(found), "позиция", "позиции", "позиций") + "\n\n")
	var rows [][]telegram.Button
	for i, p := range shown {
		fmt.Fprintf(&sb, "%d. %s — %s%s\n", i+1, bold(productTitle(p.Name, p.Brand)),
			qty(p.Unit, p.Stock.Total), placesText(p, locs))
		rows = append(rows, telegram.Row(telegram.Btn(label(productTitle(p.Name, p.Brand), 40), "p:"+strconv.FormatInt(p.ID, 10))))
	}
	if len(found) > maxResults {
		sb.WriteString("\nПозиций больше десяти — уточните запрос.")
	}
	b.send(ctx, chatID, sb.String(), &telegram.Keyboard{Rows: rows})
}

// locationNames — названия мест пользователя по id (включая архивные).
func (b *Bot) locationNames(ctx context.Context, userID int64) map[int64]string {
	out := map[int64]string{}
	list, err := b.cat.Locations(ctx, userID, true)
	if err != nil {
		b.log.Warn("telegram: места", "err", err)
		return out
	}
	for _, l := range list {
		out[l.ID] = l.Name
	}
	return out
}

// placesText — « · Полка, Коробка» по местам экземпляров позиции.
func placesText(p catalog.ProductSummary, locs map[int64]string) string {
	var names []string
	for _, id := range p.LocationIDs {
		if n, ok := locs[id]; ok {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return ""
	}
	return " · " + telegram.Esc(strings.Join(names, ", "))
}

// sendCard присылает карточку позиции (ФТ-11.3.2).
func (b *Bot) sendCard(ctx context.Context, link Link, chatID, productID int64) {
	text, kb, err := b.cardText(ctx, link.UserID, productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return
	}
	b.send(ctx, chatID, text, kb)
}

// cardText — карточка: остаток по экземплярам и местам, минимум, последняя
// цена и магазин, кнопки расхода.
func (b *Bot) cardText(ctx context.Context, userID, productID int64) (string, *telegram.Keyboard, error) {
	d, err := b.cat.Product(ctx, userID, productID)
	if err != nil {
		return "", nil, err
	}
	p := d.Product
	locs := b.locationNames(ctx, userID)

	var sb strings.Builder
	sb.WriteString(bold(productTitle(p.Name, p.Brand)) + "\n")
	if p.Stock.Total == 0 {
		sb.WriteString("Остаток: нет в наличии\n")
	} else {
		sb.WriteString("Остаток: " + qty(p.Unit, p.Stock.Total) + "\n")
		type unitGroup struct {
			status      string
			remain, cap int64
			loc         string
			n           int
		}
		var groups []*unitGroup
		for _, u := range d.Units {
			where := ""
			if u.LocationID != nil {
				where = locs[*u.LocationID]
			}
			var g *unitGroup
			for _, x := range groups {
				if x.status == u.Status && x.remain == u.RemainingQty && x.cap == u.InitialQty && x.loc == where {
					g = x
					break
				}
			}
			if g == nil {
				g = &unitGroup{status: u.Status, remain: u.RemainingQty, cap: u.InitialQty, loc: where}
				groups = append(groups, g)
			}
			g.n++
		}
		for _, g := range groups {
			kind := "запечатан"
			if g.status == "open" {
				kind = "вскрыт"
			}
			if g.n > 1 {
				kind += fmt.Sprintf(" ×%d", g.n)
			}
			line := fmt.Sprintf("  • %s: %s из %s", kind, qty(p.Unit, g.remain), qty(p.Unit, g.cap))
			if g.loc != "" {
				line += " · " + telegram.Esc(g.loc)
			}
			sb.WriteString(line + "\n")
		}
	}
	if p.MinStock != nil {
		sb.WriteString("Минимум: " + qty(p.Unit, *p.MinStock))
		if p.Low {
			sb.WriteString(" — ниже минимума")
		}
		sb.WriteString("\n")
	}
	if len(d.Purchases) > 0 {
		last := d.Purchases[0]
		sb.WriteString("Последняя цена: " + money(last.PricePerPackage))
		if last.Store != nil {
			sb.WriteString(" · " + telegram.Esc(*last.Store))
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n"), consumeKeyboard(p), nil
}

// consumeKeyboard — кнопки расхода: мл −10/−50/−100, шт −1/−5 (ФТ-11.5.2).
func consumeKeyboard(p catalog.ProductSummary) *telegram.Keyboard {
	if p.Stock.Total == 0 {
		return nil
	}
	steps := []int64{10, 50, 100}
	if p.Unit == "pcs" {
		steps = []int64{1, 5}
	}
	row := make([]telegram.Button, 0, len(steps))
	for _, n := range steps {
		row = append(row, telegram.Btn("−"+group(n), fmt.Sprintf("c:%d:%d", p.ID, n)))
	}
	return &telegram.Keyboard{Rows: [][]telegram.Button{row}}
}
