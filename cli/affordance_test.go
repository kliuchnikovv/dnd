package cli

import (
	"encoding/json"
	"reflect"
	"strconv"
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

// Номер разворачивается в тот же интент, что и написанный словами вариант. Два
// пути к одному ходу разошлись бы, и разошлись бы молча.
func TestNumberExpandsIntoTheSameIntent(t *testing.T) {
	// Вариант с осмотром ищется в наборе, а не берётся номером наугад:
	// приоритет категорий — дело ядра, и тест не обязан его повторять.
	g := renderGame(t)
	want, index := core.Intent{}, 0
	for i, a := range g.Affordances() {
		if a.Intent.Verb == "examine" {
			want, index = a.Intent, i+1
		}
	}
	if index == 0 {
		t.Fatal("в наборе нет осмотра — тест сравнивать не с чем")
	}

	byNumber := affordSession(t, strconv.Itoa(index)+"\nquit\n")
	byWords := affordSession(t, "examine "+string(want.Args.Target)+"\nquit\n")
	if !reflect.DeepEqual(byNumber.first, byWords.first) {
		t.Errorf("номер и слова дали разные ходы:\n%+v\n%+v", byNumber.first, byWords.first)
	}
	if !reflect.DeepEqual(byNumber.first, want) {
		t.Errorf("номер развернулся не в свой вариант:\n%+v\n%+v", byNumber.first, want)
	}
}

// Номер вне диапазона хода не тратит: это опечатка, а не заявка.
func TestOutOfRangeNumberSpendsNoTurn(t *testing.T) {
	run := affordSession(t, "9\nquit\n")
	if run.journalled != 0 {
		t.Errorf("номер вне диапазона попал в журнал команд: %d", run.journalled)
	}
	if !strings.Contains(run.out, "такого варианта нет") {
		t.Errorf("игрок не узнал, что промахнулся:\n%s", run.out)
	}
}

// Без включённого набора номер остаётся обычным вводом: игра без аффордансов
// обязана работать как работала.
func TestNumberIsPlainInputWithoutAffordances(t *testing.T) {
	g := renderGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader("1\nquit\n"), &out)
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	if strings.Contains(out.String(), "Что можно:") {
		t.Errorf("набор напечатан без включения:\n%s", out.String())
	}
	if strings.Contains(out.String(), "такого варианта нет") {
		t.Errorf("номер разобран как вариант при выключенном наборе:\n%s", out.String())
	}
}

// Набор печатается каждый ход: он и есть ответ на вопрос «что теперь».
func TestAffordancesArePrintedEveryTurn(t *testing.T) {
	run := affordSession(t, "survey\nsurvey\nquit\n")
	if n := strings.Count(run.out, "Что можно:"); n < 3 {
		t.Errorf("набор напечатан %d раз на старте и двух ходах:\n%s", n, run.out)
	}
}

type affordRun struct {
	out        string
	first      core.Intent
	journalled int
}

// affordSession гоняет сессию с включённым набором и подсматривает журнал
// команд: правда реплея — интент, а не строка, и номер обязан доезжать до неё.
func affordSession(t *testing.T, script string) affordRun {
	t.Helper()
	g := renderGame(t)
	var out strings.Builder
	db := g.DB
	s := NewSession(g, strings.NewReader(script), &out).
		WithJournal(NewJournal(db, "s-afford", "snap", 1)).WithAffordances()
	if err := s.Run(); err != nil {
		t.Fatalf("прогон: %v", err)
	}
	run := affordRun{out: out.String()}
	for _, e := range db.CommandLog {
		run.journalled++
		if run.journalled == 1 {
			var cmd loggedCommand
			if err := json.Unmarshal(e.Intent, &cmd); err != nil {
				t.Fatalf("интент из журнала: %v", err)
			}
			run.first = cmd.Intent
		}
	}
	return run
}

// Имя во фразе идёт без пояснения после запятой: «заговорить с Берн, стражник»
// не согласуется — пояснение стоит в именительном. В перечне целей оно
// остаётся: там оно полезно.
func TestNameInPhraseDropsTheApposition(t *testing.T) {
	if got := shortName("Берн, стражник"); got != "Берн" {
		t.Errorf("пояснение не отрезано: %q", got)
	}
	if got := shortName("Штабель бочек"); got != "Штабель бочек" {
		t.Errorf("имя без запятой пострадало: %q", got)
	}
}

// Тег не обещает проверки, которой не будет. Переход системой правил не
// бросается вовсе, и «[перемещение]» читалось бы игроком как заявка на риск.
func TestMoveOptionPromisesNoCheck(t *testing.T) {
	g := renderGame(t)
	for _, a := range g.Affordances() {
		if a.Intent.Verb != "move_zone" {
			continue
		}
		if label := affordanceLabel(g, a); strings.Contains(label, "[") {
			t.Errorf("переход обещает проверку: %q", label)
		}
	}
}
