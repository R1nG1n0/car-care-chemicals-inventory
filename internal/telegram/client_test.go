package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "123456:SECRET-token_value"

// fakeAPI — имитация Bot API: ответы по имени метода.
type fakeAPI struct {
	t       *testing.T
	handler map[string]func(body map[string]any) (any, *apiResponse)
	calls   []string
	files   map[string][]byte
}

func newFake(t *testing.T) (*fakeAPI, *Client, *httptest.Server) {
	f := &fakeAPI{t: t, handler: map[string]func(map[string]any) (any, *apiResponse){}, files: map[string][]byte{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p, ok := strings.CutPrefix(r.URL.Path, "/file/bot"+testToken+"/"); ok {
			data, ok := f.files[p]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
			return
		}
		method, ok := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
		if !ok {
			http.NotFound(w, r)
			return
		}
		f.calls = append(f.calls, method)
		raw, _ := io.ReadAll(r.Body)
		body := map[string]any{}
		_ = json.Unmarshal(raw, &body)
		h := f.handler[method]
		if h == nil {
			t.Errorf("неожиданный метод %s", method)
			return
		}
		result, fail := h(body)
		w.Header().Set("Content-Type", "application/json")
		if fail != nil {
			_ = json.NewEncoder(w).Encode(fail)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "result": result})
	}))
	t.Cleanup(srv.Close)
	return f, New(testToken, Options{BaseURL: srv.URL}), srv
}

func TestGetMeAndSend(t *testing.T) {
	f, c, _ := newFake(t)
	f.handler["getMe"] = func(map[string]any) (any, *apiResponse) {
		return map[string]any{"id": 1, "is_bot": true, "first_name": "Гараж", "username": "garage_bot"}, nil
	}
	var sent map[string]any
	f.handler["sendMessage"] = func(b map[string]any) (any, *apiResponse) {
		sent = b
		return map[string]any{"message_id": 77, "chat": map[string]any{"id": 5, "type": "private"}}, nil
	}
	me, err := c.GetMe(context.Background())
	if err != nil || me.Username != "garage_bot" {
		t.Fatalf("getMe: %+v %v", me, err)
	}
	kb := &Keyboard{Rows: [][]Button{Row(Btn("−10", "c:1:10"), Link("Сайт", "https://x.test"))}}
	m, err := c.SendMessage(context.Background(), 5, "<b>Привет</b>", kb)
	if err != nil || m.MessageID != 77 {
		t.Fatalf("sendMessage: %+v %v", m, err)
	}
	if sent["parse_mode"] != "HTML" || sent["chat_id"].(float64) != 5 || sent["text"] != "<b>Привет</b>" {
		t.Fatalf("тело: %+v", sent)
	}
	rows := sent["reply_markup"].(map[string]any)["inline_keyboard"].([]any)
	if len(rows) != 1 || len(rows[0].([]any)) != 2 {
		t.Fatalf("клавиатура: %+v", sent["reply_markup"])
	}
}

func TestGetUpdatesAndEdit(t *testing.T) {
	f, c, _ := newFake(t)
	f.handler["getUpdates"] = func(b map[string]any) (any, *apiResponse) {
		if b["offset"].(float64) != 11 || b["timeout"].(float64) != PollTimeout {
			t.Errorf("параметры: %+v", b)
		}
		return []any{map[string]any{"update_id": 11, "callback_query": map[string]any{
			"id": "cb", "from": map[string]any{"id": 9, "first_name": "Иван"}, "data": "x",
			"message": map[string]any{"message_id": 3, "chat": map[string]any{"id": 4, "type": "private"}}}}}, nil
	}
	ups, err := c.GetUpdates(context.Background(), 11)
	if err != nil || len(ups) != 1 || ups[0].CallbackQuery.Data != "x" || ups[0].CallbackQuery.Message.Chat.ID != 4 {
		t.Fatalf("getUpdates: %+v %v", ups, err)
	}
	f.handler["editMessageText"] = func(b map[string]any) (any, *apiResponse) {
		if _, ok := b["reply_markup"]; !ok {
			t.Error("при nil-клавиатуре кнопки должны сниматься явно")
		}
		return true, nil
	}
	if err := c.EditMessageText(context.Background(), 4, 3, "готово", nil); err != nil {
		t.Fatal(err)
	}
	f.handler["editMessageText"] = func(map[string]any) (any, *apiResponse) {
		return nil, &apiResponse{OK: false, ErrorCode: 400, Description: "Bad Request: message is not modified"}
	}
	if err := c.EditMessageText(context.Background(), 4, 3, "готово", nil); err != nil {
		t.Fatalf("«не изменено» — не ошибка: %v", err)
	}
}

func TestAPIErrorAndRetryAfter(t *testing.T) {
	f, c, _ := newFake(t)
	f.handler["sendMessage"] = func(map[string]any) (any, *apiResponse) {
		r := &apiResponse{OK: false, ErrorCode: 429, Description: "Too Many Requests"}
		r.Parameters.RetryAfter = 7
		return nil, r
	}
	_, err := c.SendMessage(context.Background(), 1, "x", nil)
	var ae *APIError
	if !errors.As(err, &ae) || ae.Code != 429 || ae.RetryAfter != 7 {
		t.Fatalf("ошибка: %v", err)
	}
}

func TestDownloadFile(t *testing.T) {
	f, c, _ := newFake(t)
	f.files["photos/a.jpg"] = []byte("0123456789")
	f.handler["getFile"] = func(b map[string]any) (any, *apiResponse) {
		return map[string]any{"file_id": b["file_id"], "file_size": 10, "file_path": "photos/a.jpg"}, nil
	}
	data, err := c.DownloadFile(context.Background(), "id1", 100)
	if err != nil || string(data) != "0123456789" {
		t.Fatalf("скачивание: %q %v", data, err)
	}
	if _, err := c.DownloadFile(context.Background(), "id1", 5); !errors.Is(err, ErrFileTooBig) {
		t.Fatalf("предел по file_size: %v", err)
	}
	// file_size в getFile не указан — предел всё равно держится по факту чтения.
	f.handler["getFile"] = func(map[string]any) (any, *apiResponse) {
		return map[string]any{"file_path": "photos/a.jpg"}, nil
	}
	if _, err := c.DownloadFile(context.Background(), "id1", 5); !errors.Is(err, ErrFileTooBig) {
		t.Fatalf("предел по факту: %v", err)
	}
}

// Токен не должен попадать в ошибки транспорта (url.Error содержит адрес).
func TestTokenNotInErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // порт закрыт — соединение откажет
	c := New(testToken, Options{BaseURL: url})
	_, err := c.GetMe(context.Background())
	if err == nil {
		t.Fatal("ожидалась ошибка соединения")
	}
	for _, s := range []string{err.Error(), strings.Join(chain(err), "|")} {
		if strings.Contains(s, "SECRET") || strings.Contains(s, "123456") || strings.Contains(s, "/bot") {
			t.Fatalf("в ошибке виден токен или адрес: %s", s)
		}
	}

	// Описание ошибки от сервера с эхом токена тоже затирается.
	f, c2, _ := newFake(t)
	f.handler["getMe"] = func(map[string]any) (any, *apiResponse) {
		return nil, &apiResponse{OK: false, ErrorCode: 401, Description: "Unauthorized " + testToken}
	}
	if _, err := c2.GetMe(context.Background()); err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("описание: %v", err)
	}
}

func chain(err error) []string {
	var out []string
	for err != nil {
		out = append(out, err.Error())
		err = errors.Unwrap(err)
	}
	return out
}

func TestTimeoutAndCancel(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // выполняется до srv.Close
	c := New(testToken, Options{BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.GetMe(ctx)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("ожидался таймаут без токена: %v", err)
	}
}

func TestDisplayNameAndEsc(t *testing.T) {
	if (User{FirstName: "Иван", LastName: "П"}).DisplayName() != "Иван П" ||
		(User{Username: "ivan"}).DisplayName() != "@ivan" || (User{ID: 5}).DisplayName() != "id5" {
		t.Fatal("DisplayName")
	}
	if Esc(`a<b>&"c"`) != "a&lt;b&gt;&amp;&#34;c&#34;" {
		t.Fatalf("Esc: %s", Esc(`a<b>&"c"`))
	}
}
