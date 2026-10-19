// Package transfer — экспорт и импорт данных пользователя (ФТ-10.3).
//
// Экспорт — файл JSON формата garage-export версии 1 со всеми данными
// пользователя и три таблицы CSV для Excel. Импорт принимает такой файл
// только в пустой склад, одной транзакцией. Все запросы фильтруются
// по user_id (ТБ-5); пакет не знает про HTTP.
package transfer

import "github.com/R1nG1n0/car-care-chemicals-inventory/internal/photo"

// Признаки формата файла.
const (
	FormatName    = "garage-export"
	FormatVersion = 1
)

// File — файл экспорта. id внутри файла — прежние id из базы: они нужны
// только как ключи связей внутри файла, при импорте база выдаёт новые.
// Время — секунды Unix (UTC), даты покупок — ГГГГ-ММ-ДД, деньги — копейки,
// количество — целые мл или шт (как в базе и в API).
type File struct {
	Format     string      `json:"format"`
	Version    int         `json:"version"`
	ExportedAt string      `json:"exported_at"` // RFC 3339, UTC
	User       FileUser    `json:"user"`
	Settings   *Settings   `json:"settings"`
	Categories []Category  `json:"categories"`
	Locations  []Location  `json:"locations"`
	Stores     []Store     `json:"stores"`
	Products   []Product   `json:"products"`
	Units      []Unit      `json:"units"`
	Purchases  []Purchase  `json:"purchases"`
	RecipeRuns []RecipeRun `json:"recipe_runs"`
	Movements  []Movement  `json:"movements"`
	Recipes    []Recipe    `json:"recipes"`
	Shopping   []Shopping  `json:"shopping"`
}

// FileUser — чей это файл. Пароль и сессии в файл не попадают.
type FileUser struct {
	Login string `json:"login"`
}

// Settings — шаги кнопок количества.
type Settings struct {
	StepSmall int64 `json:"step_small"`
	StepLarge int64 `json:"step_large"`
}

// Category — категория.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Sort     int64  `json:"sort"`
	Archived bool   `json:"archived"`
}

// Location — место хранения.
type Location struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Sort      int64  `json:"sort"`
	Archived  bool   `json:"archived"`
	CheckedAt *int64 `json:"checked_at"`
}

// Store — магазин.
type Store struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Archived bool   `json:"archived"`
}

// Product — позиция каталога. Photo — необязательное фото (ФТ-10.4):
// без поля (или null) позиция остаётся без фото.
type Product struct {
	ID          int64  `json:"id"`
	CategoryID  int64  `json:"category_id"`
	Name        string `json:"name"`
	Brand       string `json:"brand"`
	Unit        string `json:"unit"`
	PackageSize int64  `json:"package_size"`
	Barcode     string `json:"barcode"`
	MinStock    *int64 `json:"min_stock"`
	Note        string `json:"note"`
	Archived    bool   `json:"archived"`
	CreatedAt   int64  `json:"created_at"`
	Photo       *Photo `json:"photo,omitempty"`
}

// Photo — фото позиции в файле: тип и байты в base64. Тип при импорте
// не принимается на веру: сервер определяет его по содержимому.
type Photo struct {
	Mime string `json:"mime"`
	Data string `json:"data"`

	// Заполняется проверкой файла (Validate): декодированные байты и сведения о них.
	raw  []byte
	info photo.Info
}

// Unit — экземпляр (бутылка, пачка).
type Unit struct {
	ID             int64  `json:"id"`
	ProductID      int64  `json:"product_id"`
	PurchaseLineID *int64 `json:"purchase_line_id"`
	LocationID     *int64 `json:"location_id"`
	InitialQty     int64  `json:"initial_qty"`
	RemainingQty   int64  `json:"remaining_qty"`
	Status         string `json:"status"`
	OpenedAt       *int64 `json:"opened_at"`
	FinishedAt     *int64 `json:"finished_at"`
	CreatedAt      int64  `json:"created_at"`
}

// Purchase — покупка со строками.
type Purchase struct {
	ID        int64          `json:"id"`
	StoreID   *int64         `json:"store_id"`
	Date      string         `json:"date"`
	Note      string         `json:"note"`
	CreatedAt int64          `json:"created_at"`
	Lines     []PurchaseLine `json:"lines"`
}

// PurchaseLine — строка покупки.
type PurchaseLine struct {
	ID              int64 `json:"id"`
	ProductID       int64 `json:"product_id"`
	Packages        int64 `json:"packages"`
	PricePerPackage int64 `json:"price_per_package"`
}

// RecipeRun — одно применение шаблона расхода (группирует движения).
type RecipeRun struct {
	ID        int64  `json:"id"`
	RecipeID  *int64 `json:"recipe_id"`
	CreatedAt int64  `json:"created_at"`
}

// Movement — запись журнала движений.
type Movement struct {
	ID             int64  `json:"id"`
	UnitID         int64  `json:"unit_id"`
	Type           string `json:"type"`
	QtyDelta       int64  `json:"qty_delta"`
	FromLocationID *int64 `json:"from_location_id"`
	ToLocationID   *int64 `json:"to_location_id"`
	Reason         string `json:"reason"`
	RecipeRunID    *int64 `json:"recipe_run_id"`
	BatchID        *int64 `json:"batch_id"`
	CreatedAt      int64  `json:"created_at"`
}

// Recipe — шаблон расхода со строками.
type Recipe struct {
	ID        int64        `json:"id"`
	Name      string       `json:"name"`
	Sort      int64        `json:"sort"`
	CreatedAt int64        `json:"created_at"`
	Items     []RecipeItem `json:"items"`
}

// RecipeItem — строка шаблона.
type RecipeItem struct {
	ProductID int64 `json:"product_id"`
	Qty       int64 `json:"qty"`
	Sort      int64 `json:"sort"`
}

// Shopping — пункт списка покупок, добавленный вручную.
type Shopping struct {
	ID        int64  `json:"id"`
	ProductID *int64 `json:"product_id"`
	Title     string `json:"title"`
	DoneAt    *int64 `json:"done_at"`
	CreatedAt int64  `json:"created_at"`
}
