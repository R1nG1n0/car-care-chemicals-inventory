package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/photo"
)

const msgPhotoNotFound = "Фото не найдено"

// Photo — фото позиции (ФТ-10.4).
type Photo struct {
	Mime      string
	Data      []byte
	UpdatedAt int64
}

// PhotoError переводит ошибку проверки фото в ошибку ввода с понятным текстом.
func PhotoError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, photo.ErrTooLarge):
		return invalid("photo", "Фото больше 400 КБ")
	case errors.Is(err, photo.ErrDimensions):
		return invalid("photo", "Фото больше 4096 × 4096 px")
	case errors.Is(err, photo.ErrEmpty):
		return invalid("photo", "Фото не передано")
	}
	return invalid("photo", "Нужно фото JPEG, PNG или WebP")
}

// SetPhoto сохраняет или заменяет фото своей позиции. Чужая и несуществующая
// позиция — ErrNotFound. Метка времени растёт при каждой замене, даже в ту
// же секунду: по ней клиент сбрасывает кэш картинки.
func (s *Service) SetPhoto(ctx context.Context, userID, productID int64, data []byte, at int64) (ProductSummary, error) {
	info, err := photo.Inspect(data)
	if err != nil {
		return ProductSummary{}, PhotoError(err)
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO product_photos (user_id, product_id, mime, data, width, height, updated_at)
		SELECT user_id, id, ?, ?, ?, ?, ? FROM products WHERE user_id = ? AND id = ?
		ON CONFLICT (user_id, product_id) DO UPDATE SET mime = excluded.mime, data = excluded.data,
			width = excluded.width, height = excluded.height,
			updated_at = MAX(excluded.updated_at, product_photos.updated_at + 1)`,
		info.Mime, data, info.Width, info.Height, at, userID, productID)
	if err != nil {
		return ProductSummary{}, fmt.Errorf("сохранение фото: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ProductSummary{}, notFound(msgProductNotFound)
	}
	return s.Summary(ctx, userID, productID)
}

// Photo — фото своей позиции; чужой позиции и отсутствия фото не различает.
func (s *Service) Photo(ctx context.Context, userID, productID int64) (Photo, error) {
	var p Photo
	err := s.q.QueryRowContext(ctx, `SELECT mime, data, updated_at FROM product_photos
		WHERE user_id = ? AND product_id = ?`, userID, productID).Scan(&p.Mime, &p.Data, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Photo{}, notFound(msgPhotoNotFound)
	}
	if err != nil {
		return Photo{}, fmt.Errorf("фото позиции: %w", err)
	}
	return p, nil
}

// DeletePhoto убирает фото своей позиции. Позиция без фото — не ошибка;
// чужая или несуществующая — ErrNotFound.
func (s *Service) DeletePhoto(ctx context.Context, userID, productID int64) (ProductSummary, error) {
	sum, err := s.Summary(ctx, userID, productID)
	if err != nil {
		return ProductSummary{}, err
	}
	if _, err := s.q.ExecContext(ctx, `DELETE FROM product_photos WHERE user_id = ? AND product_id = ?`,
		userID, productID); err != nil {
		return ProductSummary{}, fmt.Errorf("удаление фото: %w", err)
	}
	sum.PhotoUpdatedAt = nil
	return sum, nil
}
