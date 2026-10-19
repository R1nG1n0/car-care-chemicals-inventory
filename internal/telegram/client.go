// Package telegram — минимальный клиент Telegram Bot API на net/http
// (ФТ-11.1): getMe, getUpdates, sendMessage, editMessageText,
// answerCallbackQuery, getFile и скачивание файла, setMyCommands.
//
// Токен бота не попадает ни в тексты ошибок, ни в журнал: адрес запроса
// содержит токен, поэтому ошибки транспорта (url.Error) очищаются.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// DefaultBaseURL — адрес Bot API.
	DefaultBaseURL = "https://api.telegram.org"
	// PollTimeout — долгий опрос getUpdates (ФТ-11.1.1), секунды.
	PollTimeout = 50

	callTimeout     = 20 * time.Second
	pollSlack       = 15 * time.Second // запас сверх PollTimeout на сеть
	maxResponseBody = 4 << 20          // ответ API крошечный; больше — ошибка
)

// Options — настройки клиента.
type Options struct {
	// BaseURL — адрес Bot API; пусто — DefaultBaseURL. Для тестов (httptest).
	BaseURL string
	// Interface — интерфейс, к которому привязываются соединения
	// (SO_BINDTODEVICE); пусто — без привязки.
	Interface string
	// HTTPClient — готовый клиент вместо собственного (тесты); Interface тогда не используется.
	HTTPClient *http.Client
}

// Client — клиент Bot API одного бота.
type Client struct {
	token string
	base  string
	hc    *http.Client
}

// New создаёт клиент.
func New(token string, opt Options) *Client {
	base := strings.TrimRight(opt.BaseURL, "/")
	if base == "" {
		base = DefaultBaseURL
	}
	hc := opt.HTTPClient
	if hc == nil {
		hc = &http.Client{Transport: newTransport(opt.Interface)}
	}
	return &Client{token: token, base: base, hc: hc}
}

// newTransport — транспорт с таймаутами и привязкой к интерфейсу.
func newTransport(iface string) *http.Transport {
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: bindControl(iface)}
	return &http.Transport{
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          4,
		ForceAttemptHTTP2:     true,
	}
}

// APIError — Telegram ответил отказом (ok=false).
type APIError struct {
	Method      string
	Code        int
	Description string
	RetryAfter  int // секунды при 429, иначе 0
}

func (e *APIError) Error() string {
	return fmt.Sprintf("telegram %s: %d %s", e.Method, e.Code, e.Description)
}

// transportError — сбой сети без адреса запроса и токена.
type transportError struct {
	method string
	msg    string
	err    error
}

func (e *transportError) Error() string { return "telegram " + e.method + ": " + e.msg }
func (e *transportError) Unwrap() error { return e.err }

// clean превращает ошибку HTTP-клиента в безопасную: *url.Error содержит
// адрес со токеном, поэтому берётся только вложенная причина, а любые следы
// токена в тексте затираются (ФТ-11.1.3).
func (c *Client) clean(method string, err error) error {
	if err == nil {
		return nil
	}
	return &transportError{method: method, msg: c.redact(innerMessage(err)), err: unwrapURL(err)}
}

func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

func innerMessage(err error) string {
	return unwrapURL(err).Error()
}

func (c *Client) redact(s string) string {
	if c.token == "" {
		return s
	}
	s = strings.ReplaceAll(s, c.token, "***")
	// Часть токена после двоеточия — секрет сама по себе.
	if i := strings.IndexByte(c.token, ':'); i >= 0 && i+1 < len(c.token) {
		s = strings.ReplaceAll(s, c.token[i+1:], "***")
	}
	return s
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	ErrorCode   int             `json:"error_code"`
	Description string          `json:"description"`
	Parameters  struct {
		RetryAfter int `json:"retry_after"`
	} `json:"parameters"`
}

// call вызывает метод Bot API с JSON-телом и разбирает result в out (может быть nil).
func (c *Client) call(ctx context.Context, method string, timeout time.Duration, params, out any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/bot"+c.token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return c.clean(method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return c.clean(method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return c.clean(method, err)
	}
	var r apiResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return &transportError{method: method, msg: fmt.Sprintf("неожиданный ответ (HTTP %d)", resp.StatusCode)}
	}
	if !r.OK {
		return &APIError{Method: method, Code: r.ErrorCode, Description: c.redact(r.Description), RetryAfter: r.Parameters.RetryAfter}
	}
	if out != nil {
		if err := json.Unmarshal(r.Result, out); err != nil {
			return &transportError{method: method, msg: "неожиданный результат"}
		}
	}
	return nil
}

// GetMe возвращает данные бота (имя для ссылок t.me/<бот>).
func (c *Client) GetMe(ctx context.Context) (User, error) {
	var u User
	err := c.call(ctx, "getMe", callTimeout, struct{}{}, &u)
	return u, err
}

// GetUpdates — долгий опрос: обновления с номера offset, ждёт до PollTimeout секунд.
func (c *Client) GetUpdates(ctx context.Context, offset int64) ([]Update, error) {
	var ups []Update
	err := c.call(ctx, "getUpdates", (PollTimeout*time.Second)+pollSlack, map[string]any{
		"offset":          offset,
		"timeout":         PollTimeout,
		"allowed_updates": []string{"message", "callback_query"},
	}, &ups)
	return ups, err
}

// Esc экранирует текст для parse_mode=HTML.
func Esc(s string) string { return html.EscapeString(s) }

type sendParams struct {
	ChatID                int64     `json:"chat_id"`
	MessageID             int64     `json:"message_id,omitempty"`
	Text                  string    `json:"text"`
	ParseMode             string    `json:"parse_mode"`
	DisableWebPagePreview bool      `json:"disable_web_page_preview"`
	ReplyMarkup           *Keyboard `json:"reply_markup,omitempty"`
}

// SendMessage отправляет сообщение (HTML: текст вызывающий экранирует через Esc).
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, kb *Keyboard) (Message, error) {
	var m Message
	err := c.call(ctx, "sendMessage", callTimeout,
		sendParams{ChatID: chatID, Text: text, ParseMode: "HTML", DisableWebPagePreview: true, ReplyMarkup: kb}, &m)
	return m, err
}

// EditMessageText меняет текст и кнопки своего сообщения. Совпадение с
// прежним содержимым («message is not modified») — не ошибка.
func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string, kb *Keyboard) error {
	p := sendParams{ChatID: chatID, MessageID: messageID, Text: text, ParseMode: "HTML",
		DisableWebPagePreview: true, ReplyMarkup: kb}
	if kb == nil {
		// Пустая клавиатура убирает кнопки: без reply_markup Telegram оставил бы прежние.
		p.ReplyMarkup = &Keyboard{Rows: [][]Button{}}
	}
	err := c.call(ctx, "editMessageText", callTimeout, p, nil)
	var ae *APIError
	if errors.As(err, &ae) && strings.Contains(ae.Description, "message is not modified") {
		return nil
	}
	return err
}

// AnswerCallbackQuery подтверждает нажатие кнопки; text — короткая всплывающая подсказка.
func (c *Client) AnswerCallbackQuery(ctx context.Context, id, text string) error {
	return c.call(ctx, "answerCallbackQuery", callTimeout,
		map[string]any{"callback_query_id": id, "text": text}, nil)
}

// SetMyCommands задаёт меню команд бота.
func (c *Client) SetMyCommands(ctx context.Context, cmds []Command) error {
	return c.call(ctx, "setMyCommands", callTimeout, map[string]any{"commands": cmds}, nil)
}

// ErrFileTooBig — файл больше допустимого предела.
var ErrFileTooBig = errors.New("файл слишком большой")

// DownloadFile скачивает файл по file_id (getFile + загрузка). Больше
// maxBytes — ErrFileTooBig, файл целиком в память не читается.
func (c *Client) DownloadFile(ctx context.Context, fileID string, maxBytes int64) ([]byte, error) {
	var f struct {
		FileSize int64  `json:"file_size"`
		FilePath string `json:"file_path"`
	}
	if err := c.call(ctx, "getFile", callTimeout, map[string]any{"file_id": fileID}, &f); err != nil {
		return nil, err
	}
	if f.FileSize > maxBytes {
		return nil, ErrFileTooBig
	}
	if f.FilePath == "" {
		return nil, &transportError{method: "getFile", msg: "нет пути к файлу"}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/file/bot"+c.token+"/"+f.FilePath, nil)
	if err != nil {
		return nil, c.clean("download", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, c.clean("download", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &transportError{method: "download", msg: fmt.Sprintf("HTTP %d", resp.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, c.clean("download", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrFileTooBig
	}
	return data, nil
}
