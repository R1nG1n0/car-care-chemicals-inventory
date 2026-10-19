package telegram

import "strconv"

// Типы Bot API — только нужные боту поля.

// User — пользователь Telegram.
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// DisplayName — имя для показа: «Имя Фамилия», иначе @username, иначе id.
func (u User) DisplayName() string {
	name := u.FirstName
	if u.LastName != "" {
		if name != "" {
			name += " "
		}
		name += u.LastName
	}
	switch {
	case name != "":
		return name
	case u.Username != "":
		return "@" + u.Username
	}
	return "id" + strconv.FormatInt(u.ID, 10)
}

// Chat — чат; Type: private, group, supergroup, channel.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// PhotoSize — один размер фото.
type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

// Document — файл, присланный как документ.
type Document struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

// Message — сообщение.
type Message struct {
	MessageID int64       `json:"message_id"`
	From      *User       `json:"from"`
	Chat      Chat        `json:"chat"`
	Text      string      `json:"text"`
	Caption   string      `json:"caption"`
	Photo     []PhotoSize `json:"photo"`
	Document  *Document   `json:"document"`
}

// CallbackQuery — нажатие кнопки под сообщением.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

// Update — входящее обновление.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message"`
	CallbackQuery *CallbackQuery `json:"callback_query"`
}

// Command — команда меню бота.
type Command struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// Button — кнопка под сообщением: либо callback_data (до 64 байт), либо ссылка.
type Button struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// Keyboard — inline-клавиатура: ряды кнопок.
type Keyboard struct {
	Rows [][]Button `json:"inline_keyboard"`
}

// Row собирает ряд кнопок.
func Row(b ...Button) []Button { return b }

// Btn — кнопка с callback_data.
func Btn(text, data string) Button { return Button{Text: text, CallbackData: data} }

// Link — кнопка-ссылка.
func Link(text, url string) Button { return Button{Text: text, URL: url} }

// MaxCallbackData — предел callback_data в байтах (Bot API).
const MaxCallbackData = 64
