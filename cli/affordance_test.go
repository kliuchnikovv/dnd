package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
)

// Вариант читается словами игрока, а не идентификаторами. Имена берутся из тех
// же таблиц, что список целей: разойдись они, игрок читал бы про «e_toke» там,
// где сцена печатает «Токе».
func TestAffordancesReadAsWords(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances())
	if strings.Contains(out, "e_toke") || strings.Contains(out, "p_crates") {
		t.Errorf("в списке идентификаторы вместо имён:\n%s", out)
	}
	if !strings.Contains(out, "Токе") {
		t.Errorf("присутствующий не назван по имени:\n%s", out)
	}
	if !strings.Contains(out, "1.") {
		t.Errorf("список не нумерован:\n%s", out)
	}
}

// Свободный ввод равноправен, и игрок обязан это видеть. Без этой строки список
// читается как закрытое меню — ровно тот тупик, из которого ветка выбиралась.
func TestAffordanceListOffersFreeText(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances())
	if !strings.Contains(out, "своими словами") {
		t.Errorf("список выглядит закрытым меню:\n%s", out)
	}
}

// Ни одна строка списка не начинается с тире: такая строка разбирается как
// прямая речь, и перепечатанный игроком вариант ушёл бы в say.
func TestAffordanceLinesAreNotMistakenForSpeech(t *testing.T) {
	g := renderGame(t)
	for _, line := range strings.Split(Render{}.Affordances(g, g.Affordances()), "\n") {
		if _, _, ok := Speech(line); ok {
			t.Errorf("строка списка разбирается как речь: %q", line)
		}
	}
}

// Тег несёт класс проверки и никогда порог. Число живёт в гейте держателя, то
// есть в данных дела: напечатать его значило бы разметить авторские цели.
func TestCheckTagCarriesClassNotThreshold(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Affordances(g, g.Affordances())
	for _, n := range []string{"10", "14", "18"} {
		if strings.Contains(out, n) {
			t.Errorf("в списке порог %s:\n%s", n, out)
		}
	}
	if !strings.Contains(out, "[расследование]") {
		t.Errorf("у бросаемого варианта нет тега класса:\n%s", out)
	}
}

// Пустой набор печатается пустой строкой, а не заголовком без списка.
func TestEmptyAffordancesPrintNothing(t *testing.T) {
	if out := (Render{}).Affordances(renderGame(t), nil); out != "" {
		t.Errorf("пустой набор напечатал %q", out)
	}
}

// Каждый класс из реестра переведён. Непереведённый класс потерял бы тег молча:
// игрок увидел бы вариант без пометки о броске.
func TestEveryRollingClassHasAWord(t *testing.T) {
	for _, def := range core.AllVerbs() {
		if !def.Rolls {
			continue
		}
		if _, ok := classWords[def.Class]; !ok {
			t.Errorf("класс %q без перевода: %s остался бы без тега", def.Class, def.Verb)
		}
	}
}
