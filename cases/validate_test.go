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

// Гарантия нужна ровно одна. Второй держатель того же факта — это
// подтверждение, а подтверждение обязано стоить броска: иначе корроборация
// собирается сама собой и кубик не участвует в расследовании вовсе.
func TestSecondMandatoryHolderOfTheSameFactIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		second := f.FactHolders[0]
		second.HolderID = "e_toke"
		second.Gate.Verbs = []string{"question"}
		f.FactHolders = append(f.FactHolders, second)
	})
	if err == nil || !strings.Contains(err.Error(), "подтверждение") {
		t.Fatalf("второй mandatory-держатель принят: %v", err)
	}
}

// Последнее звено цепочки флейвора — общий ключ на глагол. Без него проба по
// цели, у которой фактов не осталось, печатает «[examine.e_bern]» — и игрок
// видит служебную строку вместо ответа мира.
func TestVerbWithoutGenericFlavourIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { delete(f.Flavour, "examine") })
	if err == nil || !strings.Contains(err.Error(), "examine") {
		t.Fatalf("глагол без общего текста принят: %v", err)
	}
}

// Факт, стоящий за токеном правильного ответа, по определению объясняет, что
// произошло, — значит concept. Разметка ни на что не влияет в M1a, но станет
// входом валидатора генератора в M1b, и неверной она туда попасть не должна.
func TestTruthFactMarkedAsTransitionIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Facts[0].Kind = store.FactTransition })
	if err == nil || !strings.Contains(err.Error(), "concept") {
		t.Fatalf("факт правильного ответа с разметкой transition принят: %v", err)
	}
}

// Предмет, на который ссылается гейт, обязан быть добываемым. Иначе дело
// выглядит проходимым, а факт закрыт навсегда — и видно это будет на сороковой
// минуте прогона, а не на загрузке.
func TestGateOnUnobtainableItemIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Items = append(f.Items, store.Item{ID: "i_writ", Kind: "credential", Name: "Предписание"})
		f.FactHolders[0].Gate.RequiresItems = []store.ItemID{"i_writ"}
	})
	if err == nil {
		t.Fatal("гейт на недобываемый предмет прошёл валидацию")
	}
	if !strings.Contains(err.Error(), "i_writ") {
		t.Errorf("сообщение не называет предмет: %v", err)
	}
}

// Предмет в стартовом инвентаре добываем по определению.
func TestGateOnStartingItemIsAccepted(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Items = append(f.Items, store.Item{ID: "i_writ", Kind: "credential", Name: "Предписание"})
		f.StartInventory = append(f.StartInventory, "i_writ")
		f.FactHolders[0].Gate.RequiresItems = []store.ItemID{"i_writ"}
	})
	if err != nil {
		t.Errorf("гейт на стартовый предмет отвергнут: %v", err)
	}
}

// Выдаваемый фактом — тоже добываем: путь до предмета есть, он просто длиннее.
func TestGateOnItemGrantedByFactIsAccepted(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Items = append(f.Items, store.Item{ID: "i_knife", Kind: "evidence", Name: "Нож"})
		f.Facts[0].GrantsItem = "i_knife"
		f.FactHolders[0].Gate.RequiresItems = []store.ItemID{"i_knife"}
	})
	if err != nil {
		t.Errorf("гейт на предмет, выдаваемый фактом, отвергнут: %v", err)
	}
}

// Гейт на предмет, которого в деле нет вовсе, — опечатка автора.
func TestGateOnUnknownItemIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.FactHolders[0].Gate.RequiresItems = []store.ItemID{"i_нет_такого"}
	})
	if err == nil || !strings.Contains(err.Error(), "i_нет_такого") {
		t.Errorf("гейт на несуществующий предмет прошёл: %v", err)
	}
}

// Факт, выдающий предмет, которого нет в items, — та же опечатка с другой
// стороны: узнав факт, парти получила бы пустоту.
func TestFactGrantingUnknownItemIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) { f.Facts[0].GrantsItem = "i_нет_такого" })
	if err == nil || !strings.Contains(err.Error(), "i_нет_такого") {
		t.Errorf("выдача несуществующего предмета прошла: %v", err)
	}
}

// Тег у пропа теперь опечатка: инструментальность переехала в kind, и
// сообщение обязано сказать это автору, а не просто отвергнуть дело.
func TestPropToolTagIsRejectedWithAPointer(t *testing.T) {
	err := mutate(t, func(f *File) { f.Props[0].Tags = []string{"tool"} })
	if err == nil {
		t.Fatal("тег вне словаря прошёл")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("сообщение не говорит, куда переехала инструментальность: %v", err)
	}
}

// Вид предмета — закрытый словарь: опечатка делает предмет молча бесполезным.
func TestUnknownItemKindIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Items = append(f.Items, store.Item{ID: "i_x", Kind: "credentials", Name: "Бумага"})
	})
	if err == nil || !strings.Contains(err.Error(), "credentials") {
		t.Errorf("вид вне словаря прошёл: %v", err)
	}
}

// Подсказка на факт, известный парти с самого начала, не сработает НИКОГДА:
// Hint пропускает известное. Валидатор такое пропускал, и в эталонном деле
// ровно это и лежало — два теста сбились на «чутьё молчит», прежде чем
// выяснилось, что молчать оно обязано.
func TestHintAtStartFactIsRejected(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Hints = map[store.FactID]string{"f_ligature": "Посмотрите на шею."}
	})
	if err == nil || !strings.Contains(err.Error(), "f_ligature") {
		t.Errorf("бесполезная подсказка принята: %v", err)
	}
}

// Дело, где есть что добывать, обязано уметь помочь застрявшему: иначе игрок
// первой сессии закрывает консоль молча.
func TestCaseWithObtainableFactsNeedsAHint(t *testing.T) {
	err := mutate(t, func(f *File) {
		f.Facts = append(f.Facts, store.Fact{ID: "f_second", Key: "второй"})
		f.Hints = nil
	})
	if err == nil || !strings.Contains(err.Error(), "подсказ") {
		t.Errorf("дело без подсказок принято: %v", err)
	}
}

// А дело, все факты которого выданы на старте, подсказки требовать не может:
// указывать не на что. Так устроены минимальные дела для тестов.
func TestCaseWithNothingToFindNeedsNoHint(t *testing.T) {
	if err := mutate(t, func(f *File) { f.Hints = nil }); err != nil {
		t.Errorf("дело без добываемых фактов отбито за отсутствие подсказок: %v", err)
	}
}

// Подсказка не имеет права называть человека, который этого факта НЕ ДЕРЖИТ.
// Живой прогон получил «Ивар не отходит от стойки, спросите его про деньги»,
// хотя f_ivar_debt держит Сигрид: подсказка отправляла мимо цели, и это
// пережило и авторскую редактуру, и мою.
func TestHintNamingANonHolderIsRejected(t *testing.T) {
	raw, err := os.ReadFile("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	f := decodeFile(t, raw)
	// Ивар — кузнец из дела, но f_ivar_debt держит вдова, а не он.
	f.Hints["f_ivar_debt"] = "Ивар не отходит от стойки. Спросите его про деньги."

	err = validateFile(f)
	if err == nil || !strings.Contains(err.Error(), "f_ivar_debt") {
		t.Errorf("подсказка мимо держателя принята: %v", err)
	}
}

// Назвать держателя — законно: подсказка указывает именно на него.
func TestHintNamingItsOwnHolderIsAccepted(t *testing.T) {
	raw, err := os.ReadFile("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	f := decodeFile(t, raw)
	f.Hints["f_ivar_debt"] = "С Сигрид вы так и не поговорили."

	if err := validateFile(f); err != nil {
		t.Errorf("подсказка на своего держателя отбита: %v", err)
	}
}

// Оба рукописных дела обязаны проходить эту проверку как есть.
func TestHandwrittenCasesNameNoWrongHolders(t *testing.T) {
	for _, path := range []string{"../cases/harbour/case.json", "../cases/forte_merlo/case.json"} {
		if _, err := Load(path); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
}
