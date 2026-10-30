package bot

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
	"time"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/stock"
)

// pendingTTL — сколько живут ожидающие действия (отмена расхода, покупка).
const pendingTTL = 24 * time.Hour

type pendingKind int

const (
	pendUndoConsume  pendingKind = iota // отмена расхода: movementIDs
	pendPurchase                        // черновик покупки: ждёт цену
	pendUndoPurchase                    // отмена покупки: purchaseID
	pendChoice                          // выбор позиции для расхода: qty
)

// awaitTTL — сколько чат ждёт цену после «Другая цена».
const awaitTTL = 10 * time.Minute

// moveRef — движение расхода и его экземпляр: перед отменой проверяется,
// что под этим id в журнале всё ещё то же движение.
type moveRef struct{ id, unitID int64 }

// pendingEntry — действие, которое не влезает в callback_data (64 байта):
// например, id движений расхода. Хранится в памяти бота по короткому ключу.
type pendingEntry struct {
	kind       pendingKind
	userID     int64 // чей: чужой ключ не срабатывает
	moves      []moveRef
	qty        int64
	solution   *stock.Solution // расход раствором (ФТ-17), qty тогда не используется
	productID  int64
	packages   int64
	price      *int64 // цена за упаковку, копейки; nil — ещё не известна
	storeID    *int64
	storeName  string
	purchaseID int64
	chatID     int64
	msgID      int64
	expires    time.Time
}

type awaitEntry struct {
	key   string
	until time.Time
}

type pending struct {
	mu       sync.Mutex
	m        map[string]*pendingEntry
	awaiting map[int64]awaitEntry // чат → черновик, ждущий цену
	now      func() time.Time
}

func newPending(now func() time.Time) *pending {
	return &pending{m: map[string]*pendingEntry{}, awaiting: map[int64]awaitEntry{}, now: now}
}

// add сохраняет действие и возвращает короткий ключ (8 символов base64url).
func (p *pending) add(e pendingEntry) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for k, v := range p.m {
		if !v.expires.After(now) {
			delete(p.m, k)
		}
	}
	e.expires = now.Add(pendingTTL)
	for {
		raw := make([]byte, 6)
		if _, err := rand.Read(raw); err != nil {
			panic("crypto/rand: " + err.Error()) // недоступный источник случайности — не продолжаем
		}
		key := base64.RawURLEncoding.EncodeToString(raw)
		if _, taken := p.m[key]; !taken {
			p.m[key] = &e
			return key
		}
	}
}

// restore возвращает взятое действие на место (операция не удалась, можно повторить).
func (p *pending) restore(key string, e *pendingEntry) {
	p.mu.Lock()
	p.m[key] = e
	p.mu.Unlock()
}

// get возвращает действие, если оно живо и принадлежит userID.
func (p *pending) get(key string, userID int64) (*pendingEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.getLocked(key, userID)
}

func (p *pending) getLocked(key string, userID int64) (*pendingEntry, bool) {
	e, ok := p.m[key]
	if !ok || e.userID != userID || !e.expires.After(p.now()) {
		return nil, false
	}
	return e, true
}

// take — get с удалением: действие выполняется один раз.
func (p *pending) take(key string, userID int64) (*pendingEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.getLocked(key, userID)
	if ok {
		delete(p.m, key)
	}
	return e, ok
}

// setAwaiting запоминает, что следующее сообщение чата — цена для черновика key.
func (p *pending) setAwaiting(chatID int64, key string) {
	p.mu.Lock()
	p.awaiting[chatID] = awaitEntry{key: key, until: p.now().Add(awaitTTL)}
	p.mu.Unlock()
}

// clearAwaiting снимает ожидание цены в чате.
func (p *pending) clearAwaiting(chatID int64) {
	p.mu.Lock()
	delete(p.awaiting, chatID)
	p.mu.Unlock()
}

// awaitingKey — ключ черновика, которому чат должен цену.
func (p *pending) awaitingKey(chatID int64) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.awaiting[chatID]
	if ok && !a.until.After(p.now()) {
		delete(p.awaiting, chatID)
		return "", false
	}
	return a.key, ok
}
