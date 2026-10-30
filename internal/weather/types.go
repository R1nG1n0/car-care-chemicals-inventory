// Package weather — прогноз погоды Open-Meteo для мойки и покрытий (ФТ-16):
// клиент прогноза и геокодера, кэш в памяти, оценка дня и место пользователя.
package weather

import (
	"time"
	_ "time/tzdata" // зона прогноза без системной базы зон
)

// Place — место, для которого берётся прогноз.
type Place struct {
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	// Custom — место выбрал сам пользователь (а не взято из конфигурации).
	Custom bool `json:"custom"`
}

// Found — результат поиска города в геокодере.
type Found struct {
	Name    string  `json:"name"`
	Admin1  string  `json:"admin1"`
	Country string  `json:"country"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
}

// Hour — один час прогноза в местном времени места.
type Hour struct {
	Time     time.Time
	Temp     float64 // °C
	Precip   float64 // мм за час
	Prob     int     // вероятность осадков, %
	Wind     float64 // м/с
	Humidity int     // %
	Code     int     // код погоды WMO
	// NoPrecip — осадки или их вероятность в ответе пусты (null): неизвестно, а не ноль.
	NoPrecip bool
}

// Day — сутки прогноза.
type Day struct {
	Date   string // ГГГГ-ММ-ДД, местная дата места
	TMin   float64
	TMax   float64
	Precip float64
	Prob   int
	Code   int
}

// Forecast — разобранный ответ Open-Meteo.
type Forecast struct {
	// Loc — часовой пояс места (из ответа).
	Loc *time.Location
	// UTCOffset — смещение места от UTC, секунды (utc_offset_seconds ответа).
	UTCOffset int
	Hours     []Hour
	Days      []Day
}
