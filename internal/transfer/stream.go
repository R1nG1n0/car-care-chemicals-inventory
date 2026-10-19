package transfer

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// WriteJSON пишет файл экспорта потоком: список позиций идёт по одной записи,
// фото каждой позиции читается отдельным коротким запросом и сразу уходит в w.
// Всё в памяти — только снимок f без фото и одна позиция с её фото. Соединение
// с базой на время записи в w не занято (одно соединение на весь сервер).
//
// withPhotos == false — файл без фото. Ошибка посередине оставляет файл
// оборванным (заголовок ответа уже отправлен); такой файл не пройдёт импорт,
// поэтому вызывающий обязан записать её в журнал.
func (s *Service) WriteJSON(ctx context.Context, userID int64, w io.Writer, f *File, withPhotos bool) error {
	var withPhoto map[int64]bool
	if withPhotos {
		ids, err := s.photoIDs(ctx, userID)
		if err != nil {
			return err
		}
		withPhoto = ids
	}
	bw := bufio.NewWriterSize(w, 64<<10)
	enc := json.NewEncoder(bw)
	var werr error
	field := func(name string, v any) {
		if werr != nil {
			return
		}
		if _, werr = fmt.Fprintf(bw, "%q:", name); werr == nil {
			werr = enc.Encode(v)
		}
		if werr == nil {
			_, werr = bw.WriteString(",")
		}
	}
	_, werr = bw.WriteString("{")
	field("format", f.Format)
	field("version", f.Version)
	field("exported_at", f.ExportedAt)
	field("user", f.User)
	field("settings", f.Settings)
	field("categories", f.Categories)
	field("locations", f.Locations)
	field("stores", f.Stores)
	if werr == nil {
		werr = s.writeProducts(ctx, userID, bw, enc, f.Products, withPhoto)
	}
	field("units", f.Units)
	field("purchases", f.Purchases)
	field("recipe_runs", f.RecipeRuns)
	field("movements", f.Movements)
	field("recipes", f.Recipes)
	if werr == nil { // последнее поле — без запятой
		if _, werr = bw.WriteString(`"shopping":`); werr == nil {
			werr = enc.Encode(f.Shopping)
		}
	}
	if werr == nil {
		_, werr = bw.WriteString("}\n")
	}
	if werr == nil {
		werr = bw.Flush()
	}
	return werr
}

// writeProducts пишет `"products":[...],` по одной позиции.
func (s *Service) writeProducts(ctx context.Context, uid int64, bw *bufio.Writer, enc *json.Encoder,
	products []Product, withPhoto map[int64]bool) error {
	if _, err := bw.WriteString(`"products":[`); err != nil {
		return err
	}
	for i := range products {
		if i > 0 {
			if _, err := bw.WriteString(","); err != nil {
				return err
			}
		}
		p := products[i]
		if withPhoto[p.ID] {
			ph, err := s.photoOf(ctx, uid, p.ID)
			if err != nil {
				return err
			}
			p.Photo = ph
		}
		if err := enc.Encode(p); err != nil {
			return err
		}
	}
	_, err := bw.WriteString("],")
	return err
}

// photoIDs — id позиций, у которых есть фото.
func (s *Service) photoIDs(ctx context.Context, uid int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	err := each(ctx, s.db, "фото позиций", `SELECT product_id FROM product_photos WHERE user_id = ?`, []any{uid},
		func(r *sql.Rows) error {
			var id int64
			if err := r.Scan(&id); err != nil {
				return err
			}
			out[id] = true
			return nil
		})
	return out, err
}

// photoOf читает фото одной позиции; фото, удалённое за время экспорта, — nil.
func (s *Service) photoOf(ctx context.Context, uid, productID int64) (*Photo, error) {
	var mime string
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT mime, data FROM product_photos WHERE user_id = ? AND product_id = ?`,
		uid, productID).Scan(&mime, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("экспорт, фото позиции %d: %w", productID, err)
	}
	return &Photo{Mime: mime, Data: base64.StdEncoding.EncodeToString(data)}, nil
}
