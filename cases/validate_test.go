package cases

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

func decodeFile(t *testing.T, raw []byte) File {
	t.Helper()
	var f File
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("разбор эталонного дела: %v", err)
	}
	return f
}

func mutate(t *testing.T, edit func(*File)) error {
	t.Helper()
	raw, err := os.ReadFile("testdata/minimal.json")
	if err != nil {
		t.Fatal(err)
	}
	f := decodeFile(t, raw)
	edit(&f)
	return validateFile(f)
}

func TestMinimalCaseIsValid(t *testing.T) {
	if _, err := Load("testdata/minimal.json"); err != nil {
		t.Fatalf("эталонное дело не проходит валидацию: %v", err)
	}
}

func TestFactWithoutMandatoryHolderIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Mandatory = false })
	if err == nil || !strings.Contains(err.Error(), "mandatory") {
		t.Fatalf("факт без mandatory-держателя принят: %v", err)
	}
}

func TestDanglingHolderReferenceIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].HolderID = "e_nobody" })
	if err == nil || !strings.Contains(err.Error(), "e_nobody") {
		t.Fatalf("держатель-призрак принят: %v", err)
	}
}

func TestAsymmetricAdjacencyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Locations[1].Adjacent = nil })
	if err == nil || !strings.Contains(err.Error(), "смежност") {
		t.Fatalf("односторонний проход принят: %v", err)
	}
}

func TestMissingFlavourKeyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { delete(f.Flavour, "clock.suspicion.filled") })
	if err == nil || !strings.Contains(err.Error(), "clock.suspicion.filled") {
		t.Fatalf("отсутствующий ключ флейвора принят: %v", err)
	}
}

func TestTruthSlotWithoutTokenIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Truth.Why = "нет_такого_токена" })
	if err == nil || !strings.Contains(err.Error(), "why") {
		t.Fatalf("правильный ответ, недостижимый ни одним токеном, принят: %v", err)
	}
}

func TestUnknownVerbInGateIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.FactHolders[0].Gate.Verbs = []string{"hack"} })
	if err == nil || !strings.Contains(err.Error(), "hack") {
		t.Fatalf("несуществующий глагол в gate принят: %v", err)
	}
}

func TestValidatorReportsEveryViolationAtOnce(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Mandatory = false
		f.FactHolders[0].HolderID = "e_nobody"
	})
	if err == nil {
		t.Fatal("нарушения не обнаружены")
	}
	if !strings.Contains(err.Error(), "mandatory") || !strings.Contains(err.Error(), "e_nobody") {
		t.Errorf("сообщено не всё: %v", err)
	}
}

func TestDanglingRequirementReferenceIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Gate.Requires = store.RequireAll("f_nonexistent")
	})
	if err == nil || !strings.Contains(err.Error(), "f_nonexistent") {
		t.Fatalf("висячая ссылка в предпосылках принята: %v", err)
	}
}

func TestRequirementThresholdOutOfRangeIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Gate.Requires = store.RequireN(5, "f_ligature")
	})
	if err == nil || !strings.Contains(err.Error(), "порог") {
		t.Fatalf("порог больше числа предпосылок принят: %v", err)
	}
}

// Проп инертен: держать факт он не может. Проверка по типу невозможна, пока
// пропы и сущности — разные таблицы, поэтому её делает загрузчик.
func TestPropCannotHoldAFact(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Props = append(f.Props, store.SceneProp{
			ID: "p_crates", Node: "n_quay", Name: "Ящики в углу", Tags: []string{"cover"},
		})
		f.FactHolders[0].HolderID = "p_crates"
	})
	if err == nil || !strings.Contains(err.Error(), "инертны") {
		t.Fatalf("проп принят как держатель факта: %v", err)
	}
}

// Тег вне словаря модификаторов декоративен, а декоративных тегов быть не
// должно: проп обязан что-то менять в броске.
func TestPropTagOutsideVocabularyIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Props = append(f.Props, store.SceneProp{
			ID: "p_rug", Node: "n_quay", Name: "Ковёр", Tags: []string{"уютный"},
		})
	})
	if err == nil || !strings.Contains(err.Error(), "уютный") {
		t.Fatalf("декоративный тег принят: %v", err)
	}
}

func TestPropInUnknownNodeIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Props = append(f.Props, store.SceneProp{
			ID: "p_lamp", Node: "n_nowhere", Name: "Фонарь", Tags: []string{"dark"},
		})
	})
	if err == nil || !strings.Contains(err.Error(), "n_nowhere") {
		t.Fatalf("проп в несуществующем узле принят: %v", err)
	}
}

// Проп без прозы печатается как «[prop.p_rug]» и тем самым сам себя выдаёт.
func TestPropWithoutTextIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Props = append(f.Props, store.SceneProp{
			ID: "p_rug", Node: "n_quay", Name: "Ковёр", Tags: []string{"narrow"},
		})
	})
	if err == nil || !strings.Contains(err.Error(), "p_rug") {
		t.Fatalf("проп без текста принят: %v", err)
	}
}

// Без клаузы верное обвинение печатает пустоту: игрок доходит до конца и не
// получает развязки. Это дыра в DoD, а не косметика.
func TestFactBehindTruthTokenNeedsASummationClause(t *testing.T) {
	err := mutate(t, func(f *File) { f.Facts[0].SummationClause = "" })
	if err == nil || !strings.Contains(err.Error(), "клауз") {
		t.Fatalf("факт правильного ответа без клаузы принят: %v", err)
	}
}

func TestCaseWithoutAftermathIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Aftermath = "" })
	if err == nil || !strings.Contains(err.Error(), "aftermath") {
		t.Fatalf("дело без последствий принято: %v", err)
	}
}

func TestCaseWithoutColdCaseTextIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.ColdCase = "" })
	if err == nil || !strings.Contains(err.Error(), "cold_case") {
		t.Fatalf("дело без текста висяка принято: %v", err)
	}
}
