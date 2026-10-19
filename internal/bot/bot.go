package bot

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // зона Europe/Moscow без системной базы зон

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/telegram"
)

const (
	// offsetKey — ключ в bot_state: номер следующего обновления (ФТ-11.1.4).
	offsetKey = "update_offset"
	// maxBackoff — наибольшая пауза между повторами при сбоях (ФТ-11.1.2).
	maxBackoff = time.Minute
	// lowCheckEvery — как часто проверяются минимумы (ФТ-11.7.4: не дольше минуты).
	lowCheckEvery = 30 * time.Second
	// maxPhoto — предел размера скачиваемого фото.
	maxPhoto = 10 << 20
)

// API — то, что боту нужно от Telegram; реализует *telegram.Client.
type API interface {
	GetMe(ctx context.Context) (telegram.User, error)
	GetUpdates(ctx context.Context, offset int64) ([]telegram.Update, error)
	SendMessage(ctx context.Context, chatID int64, text string, kb *telegram.Keyboard) (telegram.Message, error)
	EditMessageText(ctx context.Context, chatID, messageID int64, text string, kb *telegram.Keyboard) error
	AnswerCallbackQuery(ctx context.Context, id, text string) error
	DownloadFile(ctx context.Context, fileID string, maxBytes int64) ([]byte, error)
	SetMyCommands(ctx context.Context, cmds []telegram.Command) error
}

// Options — зависимости бота.
type Options struct {
	DB  *sql.DB
	API API
	Log *slog.Logger
	// Now — часы; nil — time.Now.
	Now func() time.Time
	// SiteURL — адрес сайта для ссылки «создайте позицию» (ФТ-11.4.3); пусто —
	// без ссылки.
	SiteURL string
}

// Bot — Telegram-бот одного экземпляра garage.
type Bot struct {
	db      *sql.DB
	api     API
	log     *slog.Logger
	now     func() time.Time
	siteURL string
	links   *Links
	cat     *catalog.Service
	stk     *stock.Service
	pend    *pending
	msk     *time.Location

	// linkMu: привязка + засев минимумов против проверки минимумов.
	linkMu sync.Mutex

	mu       sync.RWMutex
	username string

	// minBackoff — первая пауза после сбоя; в тестах уменьшается.
	minBackoff time.Duration
}

// New создаёт бота. Запуск — Run.
func New(opt Options) *Bot {
	now := opt.Now
	if now == nil {
		now = time.Now
	}
	log := opt.Log
	if log == nil {
		log = slog.Default()
	}
	msk, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		msk = time.FixedZone("MSK", 3*60*60)
	}
	return &Bot{
		db: opt.DB, api: opt.API, log: log, now: now, siteURL: strings.TrimRight(opt.SiteURL, "/"),
		links: NewLinks(opt.DB, now), cat: catalog.New(opt.DB), stk: stock.New(opt.DB, now),
		pend: newPending(now), msk: msk, minBackoff: 2 * time.Second,
	}
}

// Links — хранилище привязок (общее с сайтом).
func (b *Bot) Links() *Links { return b.links }

// Enabled — бот включён. Для сайта (ФТ-11.2.1): у созданного бота всегда да.
func (b *Bot) Enabled() bool { return true }

// BotUsername — имя бота для ссылок t.me; пусто, пока getMe не удался.
func (b *Bot) BotUsername() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.username
}

func (b *Bot) setUsername(u string) {
	b.mu.Lock()
	b.username = u
	b.mu.Unlock()
}

// Run крутит бота до отмены ctx: опрос Telegram и проверка минимумов.
// Сбои сети не останавливают его — пауза растёт до минуты (ФТ-11.1.2).
func (b *Bot) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.lowLoop(ctx)
	}()
	b.pollLoop(ctx)
	wg.Wait()
}

func (b *Bot) pollLoop(ctx context.Context) {
	delay := time.Duration(0)
	fail := func(what string, err error) {
		if ctx.Err() != nil {
			return
		}
		delay = nextBackoff(delay, b.minBackoff)
		if wait := retryAfter(err); wait > delay {
			delay = min(wait, maxBackoff)
		}
		b.log.Warn("telegram: "+what, "err", err, "retry_in", delay)
		sleep(ctx, delay)
	}
	var nextCommands time.Time // когда пробовать задать меню команд
	commandsSet := false
	for ctx.Err() == nil {
		if b.BotUsername() == "" {
			me, err := b.api.GetMe(ctx)
			if err != nil {
				fail("getMe", err)
				continue
			}
			b.setUsername(me.Username)
			b.log.Info("telegram: бот подключён", "username", me.Username)
		}
		if !commandsSet && !b.now().Before(nextCommands) {
			// Меню команд не обязательно для работы: сбой не блокирует опрос.
			if err := b.api.SetMyCommands(ctx, botCommands); err != nil {
				b.log.Warn("telegram: setMyCommands", "err", err, "retry_in", time.Hour)
				nextCommands = b.now().Add(time.Hour)
			} else {
				commandsSet = true
			}
		}
		offset, err := b.loadOffset(ctx)
		if err != nil {
			fail("чтение offset", err)
			continue
		}
		ups, err := b.api.GetUpdates(ctx, offset)
		if err != nil {
			fail("getUpdates", err)
			continue
		}
		delay = 0
		for _, u := range ups {
			if ctx.Err() != nil {
				return
			}
			b.safeHandle(ctx, u)
			// Обработал — сохранил. Сбой обработки не должен зациклить бота на
			// одном обновлении, поэтому offset двигается в любом случае.
			if err := b.saveOffset(context.WithoutCancel(ctx), u.UpdateID+1); err != nil {
				b.log.Error("telegram: не удалось сохранить offset", "err", err)
			}
		}
	}
}

func nextBackoff(cur, first time.Duration) time.Duration {
	if cur == 0 {
		return first
	}
	return min(cur*2, maxBackoff)
}

func retryAfter(err error) time.Duration {
	var ae *telegram.APIError
	if errors.As(err, &ae) && ae.RetryAfter > 0 {
		return time.Duration(ae.RetryAfter) * time.Second
	}
	return 0
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// safeHandle не даёт панике в обработчике уронить службу.
func (b *Bot) safeHandle(ctx context.Context, u telegram.Update) {
	defer func() {
		if r := recover(); r != nil {
			b.log.Error("telegram: паника при обработке обновления", "update", u.UpdateID, "panic", fmt.Sprint(r))
		}
	}()
	b.HandleUpdate(ctx, u)
}

func (b *Bot) loadOffset(ctx context.Context) (int64, error) {
	var v string
	err := b.db.QueryRowContext(ctx, `SELECT value FROM bot_state WHERE key = ?`, offsetKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(v, 10, 64)
}

func (b *Bot) saveOffset(ctx context.Context, next int64) error {
	_, err := b.db.ExecContext(ctx, `INSERT INTO bot_state (key, value) VALUES (?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value`, offsetKey, strconv.FormatInt(next, 10))
	return err
}

// HandleUpdate разбирает одно обновление. Ошибки показываются пользователю
// понятным текстом, технические подробности идут в журнал.
func (b *Bot) HandleUpdate(ctx context.Context, u telegram.Update) {
	switch {
	case u.Message != nil:
		b.handleMessage(ctx, u.Message)
	case u.CallbackQuery != nil:
		b.handleCallback(ctx, u.CallbackQuery)
	}
}

// send отправляет сообщение; сбой только журналируется (бот не должен падать
// из-за недоставленного ответа).
func (b *Bot) send(ctx context.Context, chatID int64, text string, kb *telegram.Keyboard) (telegram.Message, bool) {
	m, err := b.api.SendMessage(ctx, chatID, text, kb)
	if err != nil {
		b.log.Warn("telegram: не отправлено сообщение", "err", err)
		return m, false
	}
	return m, true
}

func (b *Bot) edit(ctx context.Context, chatID, msgID int64, text string, kb *telegram.Keyboard) {
	if err := b.api.EditMessageText(ctx, chatID, msgID, text, kb); err != nil {
		b.log.Warn("telegram: не изменено сообщение", "err", err)
	}
}

// userError переводит ошибку сервисов в текст для чата. Незнакомые ошибки
// журналируются, пользователю — общая фраза.
func (b *Bot) userError(err error) string {
	var ve *stock.ValidationError
	var ce *stock.ConflictError
	var se *stock.ShortageError
	var cerr *catalog.Error
	switch {
	case stock.IsNotFound(err), errors.Is(err, catalog.ErrNotFound):
		return "Не найдено. Возможно, это уже изменили на сайте."
	case errors.As(err, &se):
		return fmt.Sprintf("Не хватает: на складе только %s. Ничего не списано.", qty(se.Unit, se.Available))
	case errors.As(err, &ve):
		return ve.Message
	case errors.As(err, &ce):
		return ce.Message
	case errors.As(err, &cerr):
		return cerr.Message
	}
	b.log.Error("telegram: ошибка операции", "err", err)
	return "Что-то пошло не так. Попробуйте ещё раз чуть позже."
}
