package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/barcode"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

// session — привязка того, кто написал. Данные показываются только в личном
// чате с привязанным аккаунтом (ФТ-11.2.3): в группе пишет «чужой» чат.
func (b *Bot) session(ctx context.Context, from *telegram.User, chat telegram.Chat) (Link, bool) {
	if from == nil || from.IsBot || chat.Type != "private" {
		return Link{}, false
	}
	link, ok, err := b.links.ByTelegram(ctx, from.ID)
	if err != nil {
		b.log.Error("telegram: чтение привязки", "err", err)
		return Link{}, false
	}
	if !ok || link.ChatID != chat.ID {
		return Link{}, false
	}
	return link, true
}

// parseCommand выделяет «/команда@бот аргумент».
func parseCommand(text string) (cmd, arg string) {
	if !strings.HasPrefix(text, "/") {
		return "", text
	}
	first, rest, _ := strings.Cut(text, " ")
	first, _, _ = strings.Cut(first, "@")
	return strings.ToLower(first), strings.TrimSpace(rest)
}

func (b *Bot) handleMessage(ctx context.Context, m *telegram.Message) {
	if m.From == nil || m.From.IsBot || m.Chat.Type != "private" {
		return // группы и каналы бот не обслуживает
	}
	text := strings.TrimSpace(m.Text)
	cmd, arg := parseCommand(text)
	if cmd != "" {
		b.pend.clearAwaiting(m.Chat.ID) // команда отменяет ожидание цены
	}
	if cmd == "/start" {
		b.cmdStart(ctx, m, arg)
		return
	}
	link, ok := b.session(ctx, m.From, m.Chat)
	if !ok {
		b.send(ctx, m.Chat.ID, hintUnlinked, nil)
		return
	}
	switch {
	case cmd == "/help":
		b.send(ctx, m.Chat.ID, helpText, nil)
	case cmd == "/list" || strings.EqualFold(text, "купить"):
		b.cmdList(ctx, link, m.Chat.ID)
	case cmd == "/weather" || strings.EqualFold(text, "погода"):
		b.cmdWeather(ctx, link, m.Chat.ID)
	case cmd != "":
		b.send(ctx, m.Chat.ID, "Такой команды нет. Вот что я умею:\n\n"+helpText, nil)
	case len(m.Photo) > 0 || (m.Document != nil && strings.HasPrefix(m.Document.MimeType, "image/")):
		b.handlePhoto(ctx, link, m)
	case text != "":
		b.handleText(ctx, link, m.Chat.ID, text)
	}
}

// cmdStart — /start: привязка по коду или приветствие.
func (b *Bot) cmdStart(ctx context.Context, m *telegram.Message, code string) {
	chatID := m.Chat.ID
	if code != "" {
		// Привязка и засев состояния минимумов — под одним замком: проверка
		// минимумов не должна увидеть привязку без засева (ФТ-11.7.3).
		b.linkMu.Lock()
		defer b.linkMu.Unlock()
		link, err := b.links.Redeem(ctx, code, m.From.ID, chatID, m.From.DisplayName())
		switch {
		case errors.Is(err, ErrBadCode):
			b.send(ctx, chatID, hintBadCode, nil)
		case err != nil:
			b.log.Error("telegram: привязка", "err", err)
			b.send(ctx, chatID, "Не получилось подключить аккаунт. Попробуйте ещё раз чуть позже.", nil)
		default:
			// Позиции, уже бывшие ниже минимума, уведомлений не вызывают (ФТ-11.7.3).
			if err := b.seedLow(ctx, link.UserID); err != nil {
				b.log.Error("telegram: начальное состояние минимумов", "err", err)
			}
			b.send(ctx, chatID, "Готово, Telegram подключён"+b.loginSuffix(ctx, link.UserID)+".\n\n"+helpText, nil)
		}
		return
	}
	if _, ok := b.session(ctx, m.From, m.Chat); ok {
		b.send(ctx, chatID, "Здравствуйте, "+telegram.Esc(m.From.DisplayName())+"!\n\n"+helpText, nil)
		return
	}
	b.send(ctx, chatID, "Здравствуйте! Я бот учёта автохимии.\n\n"+hintUnlinked, nil)
}

func (b *Bot) loginSuffix(ctx context.Context, userID int64) string {
	var login string
	if err := b.db.QueryRowContext(ctx, `SELECT login FROM users WHERE id = ?`, userID).Scan(&login); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			b.log.Warn("telegram: логин пользователя", "err", err)
		}
		return ""
	}
	return " к аккаунту " + bold(login)
}

var consumeRe = regexp.MustCompile(`^(.*\S)\s+(\d{1,6})$`)

// handleText разбирает обычный текст: цена, штрихкод цифрами, «название число», поиск.
func (b *Bot) handleText(ctx context.Context, link Link, chatID int64, text string) {
	if key, ok := b.pend.awaitingKey(chatID); ok {
		if b.handlePriceInput(ctx, link, chatID, key, text) {
			return
		}
		// Не цена — обычное сообщение: ожидание снято, разбираем как поиск, расход или штрихкод.
	}
	if code, ok := barcode.Normalize(text); ok && len(code) >= 8 && len(code) <= 14 && onlyDigits(text) {
		b.searchBarcode(ctx, link, chatID, code)
		return
	}
	// «Название число» — расход, но только если фраза целиком не является
	// названием или брендом позиции («Мойка 2 в 1» — это поиск, а не расход 1 мл).
	if m := consumeRe.FindStringSubmatch(text); m != nil {
		whole, err := b.cat.Products(ctx, link.UserID, catalog.ProductFilter{Query: text})
		if err == nil && len(whole) == 0 && b.quickConsume(ctx, link, chatID, m[1], m[2]) {
			return
		}
	}
	b.searchText(ctx, link, chatID, text)
}

// onlyDigits: код набран цифрами (пробелы и дефисы между ними допустимы).
func onlyDigits(s string) bool {
	return strings.Trim(s, "0123456789 -‐‑‒–—―− ") == ""
}

// handlePhoto распознаёт штрихкод на фото. Фото нигде не сохраняется.
func (b *Bot) handlePhoto(ctx context.Context, link Link, m *telegram.Message) {
	chatID := m.Chat.ID
	fileID := pickFile(m)
	if fileID == "" {
		b.send(ctx, chatID, "Это фото слишком большое. Пришлите поменьше или наберите цифры штрихкода.", nil)
		return
	}
	data, err := b.api.DownloadFile(ctx, fileID, maxPhoto)
	if err != nil {
		if errors.Is(err, telegram.ErrFileTooBig) {
			b.send(ctx, chatID, "Это фото слишком большое. Пришлите поменьше или наберите цифры штрихкода.", nil)
			return
		}
		b.log.Warn("telegram: скачивание фото", "err", err)
		b.send(ctx, chatID, "Не получилось получить фото. Попробуйте ещё раз.", nil)
		return
	}
	code, err := barcode.Decode(data)
	switch {
	case errors.Is(err, barcode.ErrTooBig):
		b.send(ctx, chatID, "Это фото слишком большое. Пришлите поменьше или наберите цифры штрихкода.", nil)
		return
	case errors.Is(err, barcode.ErrFormat):
		b.send(ctx, chatID, "Пришлите обычное фото (не файлом) или цифры штрихкода.", nil)
		return
	case err != nil:
		b.send(ctx, chatID, "Не вижу штрихкод. Снимите ближе и ровнее, чтобы полоски были чёткими, "+
			"или пришлите цифры под штрихкодом.", nil)
		return
	}
	b.searchBarcode(ctx, link, chatID, code)
}

// pickFile выбирает самое крупное фото не больше предела (ФТ-11.4.2); для
// документа-картинки — сам файл. Пусто — всё слишком большое.
func pickFile(m *telegram.Message) string {
	if m.Document != nil {
		if m.Document.FileSize > maxPhoto {
			return ""
		}
		return m.Document.FileID
	}
	best := ""
	var bestArea int
	for _, p := range m.Photo {
		if p.FileSize > maxPhoto {
			continue
		}
		if area := p.Width * p.Height; best == "" || area > bestArea {
			best, bestArea = p.FileID, area
		}
	}
	return best
}

func (b *Bot) notFoundHint() string {
	return "Создайте позицию на сайте или привяжите к ней этот штрихкод" + b.siteSuffix()
}

func (b *Bot) siteSuffix() string {
	if b.siteURL == "" {
		return " (Остатки → +)."
	}
	return ": " + telegram.Esc(b.siteURL)
}

func fmtCount(n int, one, few, many string) string {
	return fmt.Sprintf("%d %s", n, plural(n, one, few, many))
}
