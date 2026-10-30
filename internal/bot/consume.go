package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

// solutionText — «Раствор 1 000 мл 1:10».
func solutionText(s stock.Solution) string {
	return fmt.Sprintf("Раствор %s 1:%d", qty("ml", s.VolumeML), s.Ratio)
}

// quickConsume — «шампунь 50» (ФТ-11.5.1) или «шампунь 1000 1:10» — расход
// раствора (ФТ-17.2.5; sol != nil, qtyText — объём раствора). Возвращает
// false, если по началу фразы позиций нет — тогда вся фраза ищется как
// обычный запрос.
func (b *Bot) quickConsume(ctx context.Context, link Link, chatID int64, query, qtyText string, sol *stock.Solution) bool {
	n, err := strconv.ParseInt(qtyText, 10, 64)
	if err != nil {
		return false
	}
	if sol != nil {
		sol.VolumeML = n
	}
	found, err := b.cat.Products(ctx, link.UserID, catalog.ProductFilter{Query: query})
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return true
	}
	switch len(found) {
	case 0:
		return false
	case 1:
		b.consume(ctx, link, chatID, found[0].ID, n, sol)
	default:
		b.askWhich(ctx, link, chatID, found, n, sol)
	}
	return true
}

// askWhich — несколько совпадений: кнопки выбора позиции для списания n.
func (b *Bot) askWhich(ctx context.Context, link Link, chatID int64, found []catalog.ProductSummary, n int64, sol *stock.Solution) {
	// Выбор одноразовый: повторное нажатие по той же кнопке не спишет ещё раз.
	key := b.pend.add(pendingEntry{kind: pendChoice, userID: link.UserID, qty: n, solution: sol})
	shown := found[:min(len(found), maxResults)]
	var rows [][]telegram.Button
	for _, p := range shown {
		what := qty(p.Unit, n)
		if sol != nil {
			what = strings.ToLower(solutionText(*sol))
		}
		text := fmt.Sprintf("%s — %s", label(productTitle(p.Name, p.Brand), 34), what)
		rows = append(rows, telegram.Row(telegram.Btn(text, fmt.Sprintf("w:%s:%d", key, p.ID))))
	}
	msg := "Нашлось несколько позиций. Что списать?"
	if len(found) > maxResults {
		msg += "\nПоказаны первые десять — уточните запрос, если нужной нет."
	}
	b.send(ctx, chatID, msg, &telegram.Keyboard{Rows: rows})
}

// chooseProduct — нажатие на кнопку выбора позиции: клавиатура снимается, расход один раз.
func (b *Bot) chooseProduct(ctx context.Context, link Link, chatID, msgID int64, key string, productID int64) string {
	e, ok := b.pend.take(key, link.UserID)
	if !ok || e.kind != pendChoice {
		return staleToast
	}
	b.edit(ctx, chatID, msgID, "Выбрано, списываю…", nil)
	return b.consume(ctx, link, chatID, productID, e.qty, e.solution)
}

// consume списывает n и отвечает: сколько списано, остаток, себестоимость и
// кнопка «Отменить» (ФТ-11.5.3). sol != nil — расход раствором (n не используется).
func (b *Bot) consume(ctx context.Context, link Link, chatID, productID, n int64, sol *stock.Solution) string {
	in := stock.ConsumeInput{ProductID: productID, Qty: n}
	if sol != nil {
		in = stock.ConsumeInput{ProductID: productID, Solution: sol}
	}
	res, err := b.stk.Consume(ctx, link.UserID, in)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Не получилось"
	}
	sum, err := b.cat.Summary(ctx, link.UserID, productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Готово"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Списано %s — %s\nОстаток: %s", qty(sum.Unit, res.Consumed), bold(productTitle(sum.Name, sum.Brand)),
		qty(sum.Unit, sum.Stock.Total))
	if res.Solution != nil {
		fmt.Fprintf(&sb, "\n%s → %s концентрата", solutionText(*sol), qty("ml", res.Solution.Concentrate))
	}
	if res.Cost != nil {
		sb.WriteString("\nСебестоимость: " + money(*res.Cost))
	}
	if sum.Low {
		sb.WriteString("\nЭто ниже минимума (" + qty(sum.Unit, *sum.MinStock) + ").")
	}
	moves, err := b.moveRefs(ctx, link.UserID, res.MovementIDs)
	if err != nil {
		b.log.Error("telegram: движения расхода", "err", err)
	}
	key := b.pend.add(pendingEntry{kind: pendUndoConsume, userID: link.UserID, moves: moves, productID: productID})
	kb := &telegram.Keyboard{Rows: [][]telegram.Button{telegram.Row(telegram.Btn("Отменить", "u:"+key))}}
	b.send(ctx, chatID, sb.String(), kb)
	return "Списано " + qty(sum.Unit, res.Consumed)
}

// undoConsume — кнопка «Отменить» под ответом о расходе (обычная отмена, ФТ-5.6).
func (b *Bot) undoConsume(ctx context.Context, link Link, chatID, msgID int64, key string) string {
	e, ok := b.pend.take(key, link.UserID)
	if !ok || e.kind != pendUndoConsume {
		return staleToast
	}
	ids, same := b.sameMoves(ctx, link.UserID, e.moves)
	if !same {
		b.send(ctx, chatID, "Это действие уже отменено или изменено на сайте.", nil)
		return "Уже изменено"
	}
	if _, err := b.stk.Revert(ctx, link.UserID, ids); err != nil {
		// Отмену можно повторить: ключ возвращается на место.
		b.pend.restore(key, e)
		b.send(ctx, chatID, "Не удалось отменить. "+b.userError(err), nil)
		return "Не получилось"
	}
	text := "Расход отменён."
	if sum, err := b.cat.Summary(ctx, link.UserID, e.productID); err == nil {
		text = fmt.Sprintf("Расход отменён — %s\nОстаток: %s", bold(productTitle(sum.Name, sum.Brand)), qty(sum.Unit, sum.Stock.Total))
	}
	b.edit(ctx, chatID, msgID, text, nil)
	return "Отменено"
}

// moveRefs читает экземпляры движений только что сделанного расхода.
func (b *Bot) moveRefs(ctx context.Context, userID int64, ids []int64) ([]moveRef, error) {
	out := make([]moveRef, 0, len(ids))
	for _, id := range ids {
		var unitID int64
		if err := b.db.QueryRowContext(ctx, `SELECT unit_id FROM movements WHERE user_id = ? AND id = ?`,
			userID, id).Scan(&unitID); err != nil {
			return nil, err
		}
		out = append(out, moveRef{id: id, unitID: unitID})
	}
	return out, nil
}

// sameMoves сверяет, что под сохранёнными id всё ещё те же движения расхода:
// после отмены на сайте id могли освободиться и достаться чужой записи.
func (b *Bot) sameMoves(ctx context.Context, userID int64, moves []moveRef) ([]int64, bool) {
	if len(moves) == 0 {
		return nil, false
	}
	ids := make([]int64, 0, len(moves))
	for _, m := range moves {
		var unitID int64
		var typ string
		err := b.db.QueryRowContext(ctx, `SELECT unit_id, type FROM movements WHERE user_id = ? AND id = ?`,
			userID, m.id).Scan(&unitID, &typ)
		if err != nil || unitID != m.unitID || typ != "consume" {
			return nil, false
		}
		ids = append(ids, m.id)
	}
	return ids, true
}
