package transfer

import (
	"fmt"

	"github.com/R1nG1n0/car-care-chemicals-inventory/internal/catalog"
)

// Note — заметка (версия 4, ФТ-18.5): связи — id позиций и шаблонов из файла.
type Note struct {
	ID         int64   `json:"id"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	ProductIDs []int64 `json:"product_ids"`
	RecipeIDs  []int64 `json:"recipe_ids"`
	CreatedAt  int64   `json:"created_at"`
	UpdatedAt  int64   `json:"updated_at"`
}

// Rag — тряпка с ресурсом (версия 4, ФТ-19.6). ConsumeMovementID — id
// движения расхода из файла; null — движения нет (в файле с id, которого нет
// среди movements, импорт обнуляет связь).
type Rag struct {
	ID                int64   `json:"id"`
	ProductID         int64   `json:"product_id"`
	Label             string  `json:"label"`
	Number            int64   `json:"number"`
	Washes            int64   `json:"washes"`
	Status            string  `json:"status"`
	RetireReason      *string `json:"retire_reason"`
	ConsumeMovementID *int64  `json:"consume_movement_id"`
	CreatedAt         int64   `json:"created_at"`
	RetiredAt         *int64  `json:"retired_at"`
	LastWashedAt      *int64  `json:"last_washed_at"`
}

const maxRagWashes = 1000000

// validateNotes проверяет заметки так же, как POST /api/notes.
func validateNotes(f *File, st *sets) error {
	recipes := ids{}
	for _, r := range f.Recipes {
		recipes[r.ID] = true
	}
	seen := ids{}
	for i := range f.Notes {
		n := &f.Notes[i]
		p := func(field string) string { return at("notes", i, field) }
		if err := seen.add(p("id"), n.ID); err != nil {
			return err
		}
		var err error
		if n.Title, n.Body, err = catalog.ValidateNoteText(n.Title, n.Body); err != nil {
			return fromField(at("notes", i, ""), err)
		}
		if err := checkNoteLinks(p("product_ids"), n.ProductIDs, st.products, "Позиции"); err != nil {
			return err
		}
		if err := checkNoteLinks(p("recipe_ids"), n.RecipeIDs, recipes, "Шаблона"); err != nil {
			return err
		}
		if err := checkTime(p("created_at"), n.CreatedAt); err != nil {
			return err
		}
		if err := checkTime(p("updated_at"), n.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

func checkNoteLinks(path string, list []int64, known ids, what string) error {
	if len(list) > catalog.MaxNoteLinks {
		return bad(path, fmt.Sprintf("Связей — не больше %d", catalog.MaxNoteLinks))
	}
	dup := map[int64]bool{}
	for j, id := range list {
		p := fmt.Sprintf("%s[%d]", path, j)
		if err := known.has(p, id, what); err != nil {
			return err
		}
		if dup[id] {
			return bad(p, "Связь повторяется")
		}
		dup[id] = true
	}
	return nil
}

// validateRags проверяет тряпки: позиция в шт с ресурсом, номера по позиции
// без повторов, статус и причина списания.
func validateRags(f *File, st *sets) error {
	units := map[int64]string{}
	for _, p := range f.Products {
		units[p.ID] = p.Unit
	}
	seen := ids{}
	numbers := map[[2]int64]bool{}
	for i := range f.Rags {
		r := &f.Rags[i]
		p := func(field string) string { return at("rags", i, field) }
		if err := seen.add(p("id"), r.ID); err != nil {
			return err
		}
		if err := st.products.has(p("product_id"), r.ProductID, "Позиции"); err != nil {
			return err
		}
		if units[r.ProductID] != "pcs" {
			return bad(p("product_id"), "Тряпки бывают только у позиций в шт")
		}
		label, err := text(p("label"), r.Label, 1, 20, "Метка — от 1 до 20 символов")
		if err != nil {
			return err
		}
		r.Label = label
		if err := inRange(p("number"), r.Number, 1, 1000000, "Номер"); err != nil {
			return err
		}
		key := [2]int64{r.ProductID, r.Number}
		if numbers[key] {
			return bad(p("number"), "Номер тряпки у позиции повторяется")
		}
		numbers[key] = true
		if err := inRange(p("washes"), r.Washes, 0, maxRagWashes, "Число стирок"); err != nil {
			return err
		}
		if err := checkRagStatus(p, r); err != nil {
			return err
		}
		// Висячая ссылка (движение удалено, id переиспользован) файл не портит:
		// связь обнуляется, тряпка импортируется без неё.
		if r.ConsumeMovementID != nil && !st.movements[*r.ConsumeMovementID] {
			r.ConsumeMovementID = nil
		}
		for field, v := range map[string]*int64{"retired_at": r.RetiredAt, "last_washed_at": r.LastWashedAt} {
			if err := checkOptTime(p(field), v); err != nil {
				return err
			}
		}
		if err := checkTime(p("created_at"), r.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

func checkRagStatus(p func(string) string, r *Rag) error {
	switch r.Status {
	case "active":
		if r.RetireReason != nil || r.RetiredAt != nil {
			return bad(p("status"), "У действующей тряпки нет причины и даты списания")
		}
	case "retired":
		if r.RetiredAt == nil {
			return bad(p("retired_at"), "У списанной тряпки нужна дата списания")
		}
		if r.RetireReason != nil && *r.RetireReason != "worn" && *r.RetireReason != "lost" && *r.RetireReason != "damaged" {
			return bad(p("retire_reason"), "Причина — worn, lost или damaged")
		}
	default:
		return bad(p("status"), "Статус — active или retired")
	}
	return nil
}
