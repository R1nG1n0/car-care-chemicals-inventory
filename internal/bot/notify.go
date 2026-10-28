package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

// lowLoop раз в lowCheckEvery проверяет минимумы привязанных пользователей
// (ФТ-11.7.4: задержка не больше минуты).
func (b *Bot) lowLoop(ctx context.Context) {
	for ctx.Err() == nil {
		b.CheckLow(ctx)
		sleep(ctx, lowCheckEvery)
	}
}

// CheckLow один проход проверки минимумов для всех привязанных пользователей.
func (b *Bot) CheckLow(ctx context.Context) {
	links, err := b.links.All(ctx)
	if err != nil {
		b.log.Error("telegram: список привязок", "err", err)
		return
	}
	for _, l := range links {
		if ctx.Err() != nil {
			return
		}
		if err := b.safeCheckUserLow(ctx, l); err != nil && ctx.Err() == nil {
			b.log.Error("telegram: проверка минимумов", "user", l.UserID, "err", err)
		}
	}
}

// safeCheckUserLow не даёт панике при проверке одного пользователя остановить
// проверку остальных и сам цикл.
func (b *Bot) safeCheckUserLow(ctx context.Context, l Link) (err error) {
	b.linkMu.Lock()
	defer b.linkMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("паника: %v", r)
		}
	}()
	return b.checkUserLow(ctx, l)
}

// checkUserLow сравнивает позиции ниже минимума с уже объявленными (ФТ-11.7.2):
// новые — уведомление и запись, ушедшие — запись удаляется, чтобы следующее
// падение снова вызвало уведомление.
func (b *Bot) checkUserLow(ctx context.Context, l Link) error {
	low, err := b.cat.Products(ctx, l.UserID, catalog.ProductFilter{Filter: catalog.FilterLow})
	if err != nil {
		return err
	}
	low = withoutDoNotBuy(low)
	known, err := b.lowState(ctx, l.UserID)
	if err != nil {
		return err
	}
	current := make(map[int64]bool, len(low))
	for _, p := range low {
		current[p.ID] = true
	}
	for id := range known {
		if !current[id] {
			if _, err := b.db.ExecContext(ctx, `DELETE FROM bot_low_state WHERE user_id = ? AND product_id = ?`, l.UserID, id); err != nil {
				return fmt.Errorf("состояние минимумов: %w", err)
			}
		}
	}
	var first error
	for _, p := range low {
		if known[p.ID] {
			continue
		}
		// Состояние записывается только после отправки: если Telegram
		// недоступен, уведомление повторится на следующем проходе. Исключение —
		// чат недоступен навсегда (заблокирован, не найден): повторять бесполезно.
		if err := b.notifyLow(ctx, l, p); err != nil && !terminalSendError(err) {
			if first == nil {
				first = err
			}
			continue // сбой по одной позиции не прерывает остальные
		}
		if _, err := b.db.ExecContext(ctx, `INSERT OR IGNORE INTO bot_low_state (user_id, product_id) VALUES (?, ?)`,
			l.UserID, p.ID); err != nil {
			return fmt.Errorf("состояние минимумов: %w", err)
		}
	}
	return first
}

// withoutDoNotBuy убирает позиции «Не брать больше»: по ним нет уведомлений
// и состояние не копится (ФТ-15.4.2).
func withoutDoNotBuy(list []catalog.ProductSummary) []catalog.ProductSummary {
	out := list[:0:0]
	for _, p := range list {
		if !p.DoNotBuy {
			out = append(out, p)
		}
	}
	return out
}

// terminalSendError — Telegram отказал насовсем: бот заблокирован или чата нет.
func terminalSendError(err error) bool {
	var ae *telegram.APIError
	if !errors.As(err, &ae) {
		return false
	}
	if ae.Code == 403 {
		return true
	}
	d := strings.ToLower(ae.Description)
	return ae.Code == 400 && (strings.Contains(d, "chat not found") || strings.Contains(d, "bot was blocked"))
}

// notifyLow — сообщение «заканчивается» с кнопкой «Куплено» (ФТ-11.7.1).
func (b *Bot) notifyLow(ctx context.Context, l Link, p catalog.ProductSummary) error {
	text := fmt.Sprintf("Заканчивается: %s\nОсталось %s, минимум %s", bold(productTitle(p.Name, p.Brand)),
		qty(p.Unit, p.Stock.Total), qty(p.Unit, *p.MinStock))
	if lp, err := b.lastPurchaseOf(ctx, l.UserID, p.ID); err == nil && lp != nil {
		text += "\nПоследняя цена: " + money(lp.price)
		if lp.storeName != "" {
			text += " · " + telegram.Esc(lp.storeName)
		}
	}
	// «Есть дома для того же» (ФТ-15.5.3); сбой поиска аналогов не мешает уведомлению.
	if an, err := b.cat.Analogs(ctx, l.UserID, p.ID, catalog.ShoppingAnalogsLimit); err == nil && len(an) > 0 {
		names := make([]string, len(an))
		for i, a := range an {
			names[i] = telegram.Esc(productTitle(a.Name, a.Brand))
		}
		text += "\nЕсть дома для того же: " + strings.Join(names, ", ")
	}
	kb := &telegram.Keyboard{Rows: [][]telegram.Button{telegram.Row(telegram.Btn("Куплено", "b:"+strconv.FormatInt(p.ID, 10)))}}
	_, err := b.api.SendMessage(ctx, l.ChatID, text, kb)
	return err
}

func (b *Bot) lowState(ctx context.Context, userID int64) (map[int64]bool, error) {
	rows, err := b.db.QueryContext(ctx, `SELECT product_id FROM bot_low_state WHERE user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("состояние минимумов: %w", err)
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// seedLow заполняет состояние текущими позициями ниже минимума без
// уведомлений — при подключении бота (ФТ-11.7.3).
func (b *Bot) seedLow(ctx context.Context, userID int64) error {
	low, err := b.cat.Products(ctx, userID, catalog.ProductFilter{Filter: catalog.FilterLow})
	if err != nil {
		return err
	}
	low = withoutDoNotBuy(low)
	return inTx(ctx, b.db, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM bot_low_state WHERE user_id = ?`, userID); err != nil {
			return err
		}
		for _, p := range low {
			if _, err := tx.ExecContext(ctx, `INSERT INTO bot_low_state (user_id, product_id) VALUES (?, ?)`, userID, p.ID); err != nil {
				return err
			}
		}
		return nil
	})
}
