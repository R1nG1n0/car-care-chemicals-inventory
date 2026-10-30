package bot

import (
	"context"
	"strconv"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

// handleCallback разбирает нажатие кнопки. Привязка проверяется на каждом
// нажатии, user_id всегда берётся из неё, а не из callback_data.
func (b *Bot) handleCallback(ctx context.Context, cb *telegram.CallbackQuery) {
	answer := func(text string) {
		if err := b.api.AnswerCallbackQuery(ctx, cb.ID, text); err != nil {
			b.log.Warn("telegram: ответ на нажатие", "err", err)
		}
	}
	if cb.Message == nil {
		answer("")
		return
	}
	chat := cb.Message.Chat
	from := cb.From
	link, ok := b.session(ctx, &from, chat)
	if !ok {
		answer("Сначала подключите бота на сайте")
		if chat.Type == "private" {
			b.send(ctx, chat.ID, hintUnlinked, nil)
		}
		return
	}
	chatID, msgID := chat.ID, cb.Message.MessageID
	kind, rest, _ := strings.Cut(cb.Data, ":")
	args := strings.Split(rest, ":")
	num := func(i int) (int64, bool) {
		if i >= len(args) {
			return 0, false
		}
		n, err := strconv.ParseInt(args[i], 10, 64)
		return n, err == nil && n > 0
	}

	switch kind {
	case "p": // карточка позиции из списка результатов
		if id, ok := num(0); ok {
			b.sendCard(ctx, link, chatID, id)
			answer("")
			return
		}
	case "c": // расход: позиция и количество
		id, ok1 := num(0)
		n, ok2 := num(1)
		if ok1 && ok2 {
			answer(b.consume(ctx, link, chatID, id, n, nil))
			return
		}
	case "w": // выбор позиции для расхода: ключ и позиция
		if len(args) == 2 {
			if id, ok := num(1); ok {
				answer(b.chooseProduct(ctx, link, chatID, msgID, args[0], id))
				return
			}
		}
	case "u":
		answer(b.undoConsume(ctx, link, chatID, msgID, rest))
		return
	case "b": // «Куплено» у позиции ниже минимума
		if id, ok := num(0); ok {
			answer(b.askPackages(ctx, link, chatID, msgID, id))
			return
		}
	case "n": // число упаковок
		id, ok1 := num(0)
		n, ok2 := num(1)
		if ok1 && ok2 {
			answer(b.askPrice(ctx, link, chatID, msgID, id, n))
			return
		}
	case "k":
		answer(b.confirmPrice(ctx, link, chatID, msgID, rest))
		return
	case "o":
		answer(b.otherPrice(ctx, link, chatID, msgID, rest))
		return
	case "d":
		answer(b.undoPurchase(ctx, link, chatID, msgID, rest))
		return
	case "m", "r": // ручной пункт: вычеркнуть / вернуть
		if id, ok := num(0); ok {
			answer(b.manualDone(ctx, link, chatID, msgID, id, kind == "m"))
			return
		}
	}
	answer("Эта кнопка устарела")
}
