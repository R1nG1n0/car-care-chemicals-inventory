package weather

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Places — место пользователя в базе (таблица weather_places).
type Places struct{ db *sql.DB }

// NewPlaces создаёт хранилище мест.
func NewPlaces(db *sql.DB) *Places { return &Places{db: db} }

// Get возвращает своё место пользователя; ok=false — не выбрано.
func (p *Places) Get(ctx context.Context, userID int64) (pl Place, ok bool, err error) {
	err = p.db.QueryRowContext(ctx, `SELECT name, lat, lon FROM weather_places WHERE user_id = ?`, userID).
		Scan(&pl.Name, &pl.Lat, &pl.Lon)
	if errors.Is(err, sql.ErrNoRows) {
		return Place{}, false, nil
	}
	if err != nil {
		return Place{}, false, fmt.Errorf("чтение места погоды: %w", err)
	}
	pl.Custom = true
	return pl, true, nil
}

// Set сохраняет место пользователя (заменяя прежнее).
func (p *Places) Set(ctx context.Context, userID int64, pl Place) error {
	_, err := p.db.ExecContext(ctx, `INSERT INTO weather_places (user_id, name, lat, lon) VALUES (?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET name = excluded.name, lat = excluded.lat, lon = excluded.lon`,
		userID, pl.Name, pl.Lat, pl.Lon)
	if err != nil {
		return fmt.Errorf("сохранение места погоды: %w", err)
	}
	return nil
}

// Delete сбрасывает место к месту по умолчанию.
func (p *Places) Delete(ctx context.Context, userID int64) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM weather_places WHERE user_id = ?`, userID); err != nil {
		return fmt.Errorf("сброс места погоды: %w", err)
	}
	return nil
}
