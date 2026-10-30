package stock

import "database/sql"

// Пределы раствора (ФТ-17): объём готового раствора и пропорция 1:N.
const (
	maxSolutionML = 100000
	maxRatio      = 1000
)

// Solution — расход объёмом готового раствора: VolumeML мл раствора с
// пропорцией 1:Ratio (Ratio частей воды на 1 часть концентрата).
type Solution struct {
	VolumeML int64 `json:"volume_ml"`
	Ratio    int64 `json:"ratio"`
}

// SolutionInfo — раствор с рассчитанным концентратом (ответ расхода).
type SolutionInfo struct {
	VolumeML    int64 `json:"volume_ml"`
	Ratio       int64 `json:"ratio"`
	Concentrate int64 `json:"concentrate"`
}

// Concentrate — сколько мл концентрата нужно на volumeML мл раствора
// с пропорцией 1:ratio: volume / (ratio + 1), к ближайшему целому
// (половины вверх), не меньше 1 мл (ФТ-17.2.2).
func Concentrate(volumeML, ratio int64) int64 {
	d := ratio + 1
	c := (2*volumeML + d) / (2 * d)
	return max(c, 1)
}

// validate проверяет диапазоны; ошибка относится к полю solution.
func (s Solution) validate() error {
	if s.VolumeML < 1 || s.VolumeML > maxSolutionML {
		return &ValidationError{Field: "solution", Message: "Объём раствора — от 1 до 100 000 мл"}
	}
	if s.Ratio < 1 || s.Ratio > maxRatio {
		return &ValidationError{Field: "solution", Message: "Пропорция — 1:N, N от 1 до 1000"}
	}
	return nil
}

// resolve проверяет раствор для позиции p и считает концентрат. Раствор
// возможен только у позиции в мл.
func (s Solution) resolve(p productInfo) (int64, error) {
	if err := s.validate(); err != nil {
		return 0, err
	}
	if p.Unit != "ml" {
		return 0, &ValidationError{Field: "solution", Message: "Раствор возможен только для позиций в мл"}
	}
	return Concentrate(s.VolumeML, s.Ratio), nil
}

func (s Solution) info(concentrate int64) *SolutionInfo {
	return &SolutionInfo{VolumeML: s.VolumeML, Ratio: s.Ratio, Concentrate: concentrate}
}

// nulls — значения для колонок solution_ml и solution_ratio.
func (s *Solution) nulls() (ml, ratio sql.NullInt64) {
	if s == nil {
		return
	}
	return sql.NullInt64{Int64: s.VolumeML, Valid: true}, sql.NullInt64{Int64: s.Ratio, Valid: true}
}

// solutionFrom собирает раствор из колонок; оба NULL — раствора нет.
func solutionFrom(ml, ratio sql.NullInt64) *Solution {
	if !ml.Valid || !ratio.Valid {
		return nil
	}
	return &Solution{VolumeML: ml.Int64, Ratio: ratio.Int64}
}
