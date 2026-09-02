package dnd5e

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func TestSavePassWithProficiency(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2, Saves: []string{"dex"}}
	// 8 + 3 + 2 = 13 против DC 13 — успех на грани.
	res := save(s, "dex", 13, 8)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("успех ожидался, получен %v", res.Class)
	}
	if res.Margin != 0 {
		t.Fatalf("margin: ждали 0, получили %d", res.Margin)
	}
}

func TestSaveFailWithoutProficiency(t *testing.T) {
	s := Sheet{Dex: 16, Prof: 2} // dex не в списке спасов
	// 8 + 3 = 11 против DC 13 — провал без prof-бонуса.
	res := save(s, "dex", 13, 8)
	if res.Class != core.OutcomeFail {
		t.Fatalf("провал ожидался, получен %v", res.Class)
	}
	if res.Margin != 2 {
		t.Fatalf("margin: ждали 2, получили %d", res.Margin)
	}
}

// TestSystemResolveDispatchesSave проверяет весь путь через System.Resolve
// для verb класса ClassSave (в реестре ядра пока нет verb с этим классом —
// используем "strike" как заглушку интента и подменяем класс напрямую через
// resolveSave — но раз System.Resolve диспетчерит по def.Class из реестра,
// а верб с ClassSave пока не заведён, тестируем прямой путь resolveSave).
func TestResolveSaveDirect(t *testing.T) {
	s := Sheet{Con: 14, Prof: 2, Saves: []string{"con"}}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	view := core.SceneView{Sheet: raw, DC: 14, SaveAbility: "con"}
	sheet, err := ParseSheet(view.Sheet)
	if err != nil {
		t.Fatal(err)
	}
	d := dice.Fixed(10) // 10 + 2(con mod) + 2(prof) = 14 vs DC 14 — успех
	res := resolveSave(sheet, core.Intent{Verb: "strike"}, view, d)
	if res.Class != core.OutcomeSuccess {
		t.Fatalf("успех ожидался, получен %v", res.Class)
	}
}
