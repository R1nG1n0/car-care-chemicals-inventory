package transfer

import (
	"errors"
	"strings"
	"testing"
)

func okFile() *File { return &File{Format: FormatName, Version: FormatVersion} }

func TestValidateLimitsPerKind(t *testing.T) {
	cases := map[string]func(f *File){
		"movements":   func(f *File) { f.Movements = make([]Movement, maxMovements+1) },
		"units":       func(f *File) { f.Units = make([]Unit, maxUnits+1) },
		"products":    func(f *File) { f.Products = make([]Product, maxProducts+1) },
		"purchases":   func(f *File) { f.Purchases = make([]Purchase, maxPurchases+1) },
		"categories":  func(f *File) { f.Categories = make([]Category, maxOtherRecords+1) },
		"shopping":    func(f *File) { f.Shopping = make([]Shopping, maxOtherRecords+1) },
		"recipe_runs": func(f *File) { f.RecipeRuns = make([]RecipeRun, maxOtherRecords+1) },
	}
	for name, mk := range cases {
		f := okFile()
		mk(f)
		var ie *ImportError
		err := Validate(f)
		if !errors.As(err, &ie) || ie.Path != name || !strings.Contains(ie.Message, "Слишком много") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// На самом пределе проверка по лимитам проходит (дальше файл отклонит содержимое).
	f := okFile()
	f.Movements = make([]Movement, maxMovements)
	var ie *ImportError
	if err := Validate(f); !errors.As(err, &ie) || ie.Path == "movements" {
		t.Fatalf("на пределе: %v", err)
	}
}

func TestValidateSortRange(t *testing.T) {
	f := okFile()
	f.Categories = []Category{{ID: 1, Name: "Мойка", Color: "blue", Sort: maxSort + 1}}
	var ie *ImportError
	if err := Validate(f); !errors.As(err, &ie) || ie.Path != "categories[0].sort" {
		t.Fatalf("sort категории: %v", err)
	}
	f = okFile()
	f.Locations = []Location{{ID: 1, Name: "Полка", Sort: -maxSort - 1}}
	if err := Validate(f); !errors.As(err, &ie) || ie.Path != "locations[0].sort" {
		t.Fatalf("sort места: %v", err)
	}
}

func TestBeginImportAllowsOne(t *testing.T) {
	s := &Service{}
	release, ok := s.BeginImport()
	if !ok {
		t.Fatal("первый импорт не начался")
	}
	if _, ok := s.BeginImport(); ok {
		t.Fatal("второй импорт начался одновременно с первым")
	}
	release()
	if release2, ok := s.BeginImport(); !ok {
		t.Fatal("после завершения новый импорт не начался")
	} else {
		release2()
	}
}
