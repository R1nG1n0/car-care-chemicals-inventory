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

// maxPriceKopecks — наибольшая цена упаковки (договор API): 1 000 000 ₽.
const maxPriceKopecks = 100_000_000

const (
	// maxListLow и maxListManual держат список в пределах сообщения Telegram
	// (4096 символов, 100 кнопок).
	maxListLow    = 25
	maxListManual = 15
)

func (b *Bot) cmdList(ctx context.Context, link Link, chatID int64) {
	text, kb, err := b.listView(ctx, link.UserID, nil)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return
	}
	b.send(ctx, chatID, text, kb)
}

// listView — список покупок (ФТ-11.6.1): позиции ниже минимума с последней
// ценой и магазином, затем ручные пункты. extra — дополнительный ряд кнопок
// (например, «Вернуть»).
func (b *Bot) listView(ctx context.Context, userID int64, extra []telegram.Button) (string, *telegram.Keyboard, error) {
	list, err := b.cat.Shopping(ctx, userID, b.now().Unix())
	if err != nil {
		return "", nil, err
	}
	var manual []catalog.ShoppingItem
	for _, it := range list.Manual {
		if it.DoneAt == nil {
			manual = append(manual, it)
		}
	}
	var sb strings.Builder
	var rows [][]telegram.Button
	if len(list.Low) == 0 && len(manual) == 0 {
		sb.WriteString("Покупать пока нечего: всё в запасе.")
	}
	if len(list.Low) > 0 {
		sb.WriteString(bold("Ниже минимума") + "\n")
		for i, it := range list.Low {
			if i >= maxListLow {
				sb.WriteString(fmt.Sprintf("…и ещё %d. Остальные смотрите на сайте.\n", len(list.Low)-maxListLow))
				break
			}
			p := it.Product
			fmt.Fprintf(&sb, "• %s — %s из %s", bold(productTitle(p.Name, p.Brand)), qty(p.Unit, p.Stock.Total), qty(p.Unit, *p.MinStock))
			if it.LastPrice != nil {
				sb.WriteString("; " + money(*it.LastPrice))
				if it.LastStore != nil {
					sb.WriteString(" · " + telegram.Esc(*it.LastStore))
				}
			}
			sb.WriteString("\n")
			rows = append(rows, telegram.Row(telegram.Btn("Куплено: "+label(p.Name, 30), "b:"+strconv.FormatInt(p.ID, 10))))
		}
	}
	if len(manual) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(bold("Ещё в списке") + "\n")
		for i, it := range manual {
			if i >= maxListManual {
				sb.WriteString(fmt.Sprintf("…и ещё %d.\n", len(manual)-maxListManual))
				break
			}
			name := manualName(it)
			sb.WriteString("• " + telegram.Esc(name) + "\n")
			rows = append(rows, telegram.Row(telegram.Btn("Куплено: "+label(name, 30), "m:"+strconv.FormatInt(it.ID, 10))))
		}
	}
	if len(extra) > 0 {
		rows = append(rows, extra)
	}
	var kb *telegram.Keyboard
	if len(rows) > 0 {
		kb = &telegram.Keyboard{Rows: rows}
	}
	return strings.TrimRight(sb.String(), "\n"), kb, nil
}

func manualName(it catalog.ShoppingItem) string {
	if it.Product != nil {
		return productTitle(it.Product.Name, it.Product.Brand)
	}
	return it.Title
}

// manualDone вычёркивает ручной пункт (ФТ-11.6.2) и перерисовывает список с
// кнопкой «Вернуть».
func (b *Bot) manualDone(ctx context.Context, link Link, chatID, msgID, itemID int64, done bool) string {
	it, err := b.cat.SetShoppingDone(ctx, link.UserID, itemID, done, b.now().Unix())
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Не найдено"
	}
	var extra []telegram.Button
	reply := "Возвращено в список"
	if done {
		extra = telegram.Row(telegram.Btn("Вернуть: "+label(manualName(it), 30), "r:"+strconv.FormatInt(itemID, 10)))
		reply = "Вычеркнуто"
	}
	text, kb, err := b.listView(ctx, link.UserID, extra)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return reply
	}
	if done {
		text += "\n\nВычеркнуто: " + telegram.Esc(manualName(it))
	}
	b.edit(ctx, chatID, msgID, text, kb)
	return reply
}

// askPackages — «Куплено» у позиции ниже минимума: сколько упаковок (ФТ-11.6.3).
func (b *Bot) askPackages(ctx context.Context, link Link, chatID, msgID, productID int64) string {
	p, err := b.cat.Summary(ctx, link.UserID, productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Не найдено"
	}
	var row []telegram.Button
	for n := 1; n <= 3; n++ {
		row = append(row, telegram.Btn(strconv.Itoa(n), fmt.Sprintf("n:%d:%d", productID, n)))
	}
	text := fmt.Sprintf("%s\nСколько упаковок по %s купили?", bold(productTitle(p.Name, p.Brand)), qty(p.Unit, p.PackageSize))
	b.editOrSend(ctx, chatID, msgID, text, &telegram.Keyboard{Rows: [][]telegram.Button{row}})
	return ""
}

// editOrSend меняет сообщение с кнопкой; нет сообщения — шлёт новое.
func (b *Bot) editOrSend(ctx context.Context, chatID, msgID int64, text string, kb *telegram.Keyboard) {
	if msgID != 0 {
		b.edit(ctx, chatID, msgID, text, kb)
		return
	}
	b.send(ctx, chatID, text, kb)
}

// askPrice — упаковки выбраны: предложить последнюю цену или попросить ввести.
func (b *Bot) askPrice(ctx context.Context, link Link, chatID, msgID, productID, packages int64) string {
	if packages < 1 || packages > 3 {
		return "Неверное число"
	}
	p, err := b.cat.Summary(ctx, link.UserID, productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Не найдено"
	}
	lp, err := b.lastPurchaseOf(ctx, link.UserID, productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return "Ошибка"
	}
	e := pendingEntry{kind: pendPurchase, userID: link.UserID, productID: productID, packages: packages,
		chatID: chatID, msgID: msgID}
	title := bold(productTitle(p.Name, p.Brand)) + fmt.Sprintf(", %d упак.", packages)
	if lp == nil {
		key := b.pend.add(e)
		b.pend.setAwaiting(chatID, key)
		b.editOrSend(ctx, chatID, msgID, title+"\nПрошлых покупок нет. Напишите цену за упаковку в рублях, например 1190 или 1190,50.", nil)
		return ""
	}
	e.price, e.storeID, e.storeName = &lp.price, lp.storeID, lp.storeName
	key := b.pend.add(e)
	text := title + "\nЦена: " + money(lp.price)
	if lp.storeName != "" {
		text += " · " + telegram.Esc(lp.storeName)
	}
	kb := &telegram.Keyboard{Rows: [][]telegram.Button{telegram.Row(
		telegram.Btn("Подтвердить", "k:"+key), telegram.Btn("Другая цена", "o:"+key))}}
	b.editOrSend(ctx, chatID, msgID, text, kb)
	return ""
}

// otherPrice — «Другая цена»: следующее сообщение чата — цена в рублях.
func (b *Bot) otherPrice(ctx context.Context, link Link, chatID, msgID int64, key string) string {
	e, ok := b.pend.get(key, link.UserID)
	if !ok || e.kind != pendPurchase {
		return staleToast
	}
	b.pend.setAwaiting(chatID, key)
	b.edit(ctx, chatID, msgID, "Напишите цену за упаковку в рублях, например 1190 или 1190,50.", nil)
	return ""
}

// handlePriceInput — текст, пока ждём цену. true — сообщение обработано как
// цена; false — это не цена, ожидание снято и текст нужно разобрать обычным
// образом.
func (b *Bot) handlePriceInput(ctx context.Context, link Link, chatID int64, key, text string) bool {
	e, ok := b.pend.get(key, link.UserID)
	if !ok || e.kind != pendPurchase {
		b.pend.clearAwaiting(chatID)
		return false
	}
	price, ok := parseRubles(text)
	if !ok {
		b.pend.clearAwaiting(chatID)
		return false
	}
	if price > maxPriceKopecks {
		b.send(ctx, chatID, "Цена — не больше 1 000 000 ₽. Напишите ещё раз.", nil)
		return true
	}
	b.pend.clearAwaiting(chatID)
	b.pend.take(key, link.UserID)
	e.price = &price
	b.buy(ctx, link, chatID, 0, e)
	return true
}

// confirmPrice — «Подтвердить» последнюю цену.
func (b *Bot) confirmPrice(ctx context.Context, link Link, chatID, msgID int64, key string) string {
	e, ok := b.pend.take(key, link.UserID)
	if !ok || e.kind != pendPurchase || e.price == nil {
		return staleToast
	}
	b.pend.clearAwaiting(chatID)
	b.buy(ctx, link, chatID, msgID, e)
	return ""
}

// buy записывает покупку (editMsgID — сообщение, которое заменить итогом; 0 — новое): дата — сегодня по Москве, магазин — как в прошлой
// покупке, место — по умолчанию (место последнего экземпляра позиции), цена —
// за упаковку (ФТ-11.6.3). Ответ с кнопкой «Отменить» (ФТ-11.6.4).
func (b *Bot) buy(ctx context.Context, link Link, chatID, editMsgID int64, e *pendingEntry) {
	in := stock.PurchaseInput{
		Date:    b.now().In(b.msk).Format("2006-01-02"),
		StoreID: e.storeID,
		Lines:   []stock.PurchaseLineInput{{ProductID: e.productID, Packages: e.packages, PricePerPackage: *e.price}},
	}
	id, err := b.stk.CreatePurchase(ctx, link.UserID, in)
	if err != nil {
		b.editOrSend(ctx, chatID, editMsgID, b.userError(err), nil)
		return
	}
	sum, err := b.cat.Summary(ctx, link.UserID, e.productID)
	if err != nil {
		b.send(ctx, chatID, b.userError(err), nil)
		return
	}
	text := fmt.Sprintf("Куплено: %s × %d\nСумма: %s", bold(productTitle(sum.Name, sum.Brand)), e.packages,
		money(e.packages**e.price))
	if e.storeName != "" {
		text += " · " + telegram.Esc(e.storeName)
	}
	text += "\nОстаток теперь: " + qty(sum.Unit, sum.Stock.Total)
	key := b.pend.add(pendingEntry{kind: pendUndoPurchase, userID: link.UserID, purchaseID: id, productID: e.productID})
	kb := &telegram.Keyboard{Rows: [][]telegram.Button{telegram.Row(telegram.Btn("Отменить", "d:"+key))}}
	b.editOrSend(ctx, chatID, editMsgID, text, kb)
}

// undoPurchase — «Отменить» покупку: удаляется целиком, пока экземпляры не
// тронуты (ФТ-11.6.4).
func (b *Bot) undoPurchase(ctx context.Context, link Link, chatID, msgID int64, key string) string {
	e, ok := b.pend.take(key, link.UserID)
	if !ok || e.kind != pendUndoPurchase {
		return staleToast
	}
	if err := b.stk.DeletePurchase(ctx, link.UserID, e.purchaseID); err != nil {
		if !stock.IsNotFound(err) {
			b.pend.restore(key, e) // можно попробовать позже (например, после отмены расхода)
		}
		b.send(ctx, chatID, b.userError(err), nil)
		return "Нельзя отменить"
	}
	text := "Покупка отменена."
	if sum, err := b.cat.Summary(ctx, link.UserID, e.productID); err == nil {
		text = fmt.Sprintf("Покупка отменена — %s\nОстаток: %s", bold(productTitle(sum.Name, sum.Brand)), qty(sum.Unit, sum.Stock.Total))
	}
	b.edit(ctx, chatID, msgID, text, nil)
	return "Покупка отменена"
}
