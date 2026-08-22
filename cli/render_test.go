package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

func renderGame(t *testing.T) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../cases/testdata/minimal.json")
	if err != nil {
		t.Fatalf("загрузка дела: %v", err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	return core.NewGame(*cfg)
}

func refusedResult(msg string) core.TurnResult {
	return core.TurnResult{Refused: true, Refusal: msg}
}

func TestRefusalReadsDifferentlyFromFailure(t *testing.T) {
	// Игрок обязан мгновенно видеть разницу: отказ не потратил ход.
	g := renderGame(t)
	r := Render{}
	refusal := r.Turn(g, core.Intent{}, refusedResult("парти об этом ничего не знает"))
	if !strings.Contains(refusal, "нельзя") {
		t.Errorf("отказ не помечен как отказ: %q", refusal)
	}
	if strings.Contains(refusal, "ПРОВАЛ") {
		t.Errorf("отказ подан как провал: %q", refusal)
	}
}

func TestFactsShowSourcesAndConfidence(t *testing.T) {
	g := renderGame(t)
	g.K.Learn("f_ligature", "e_body")
	g.K.Learn("f_ligature", "e_toke")
	out := Render{}.Facts(g)
	if !strings.Contains(out, "0.75") {
		t.Errorf("confidence не показан: %q", out)
	}
	if !strings.Contains(out, "e_body") || !strings.Contains(out, "e_toke") {
		t.Errorf("источники не показаны: %q", out)
	}
}

func TestStateShowsAttemptsAndGrit(t *testing.T) {
	g := renderGame(t)
	out := Render{}.State(g)
	for _, want := range []string{"узел", "grit", "попыт"} {
		if !strings.Contains(strings.ToLower(out), want) {
			t.Errorf("в state нет %q: %q", want, out)
		}
	}
}

func TestRenderNeverPrintsTruth(t *testing.T) {
	g := renderGame(t)
	all := Render{}.Scene(g) + Render{}.Facts(g) + Render{}.State(g) + Render{}.Clocks(g)
	for _, leak := range []string{"toke_is_killer", "<redacted>"} {
		if strings.Contains(all, leak) {
			t.Errorf("вывод содержит %q", leak)
		}
	}
}

func TestHelpListsEveryPlayerCommand(t *testing.T) {
	out := Render{}.Help()
	for _, cmd := range []string{"question", "examine", "search", "compare",
		"cross_reference", "stake_out", "theorize", "accuse", "facts", "state", "clocks"} {
		if !strings.Contains(out, cmd) {
			t.Errorf("help не упоминает %q", cmd)
		}
	}
}

// survey — единственный механизм под критерий гейта «понятно ли, что делать
// дальше, без подсказки». Список обязан быть честным: холдеры и пропы в одном
// перечне, без разметки. Размеченный список — это карта решения.
func TestSurveyListsPropsAndHoldersWithoutMarking(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Survey(g)

	for _, want := range []string{"Тело Халдена", "Ящики у стены", "Погасший фонарь"} {
		if !strings.Contains(out, want) {
			t.Errorf("в survey нет цели %q: %q", want, out)
		}
	}

	prefixes := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "·") {
			continue
		}
		prefixes[line[:strings.Index(line, "·")+len("·")]] = true
	}
	if len(prefixes) != 1 {
		t.Errorf("цели размечены по-разному, список выдаёт граф: %q", out)
	}
}

// Порядок произвольный, но стабильный: скриптовый прогон обязан быть
// воспроизводим, а игрок не должен видеть, как список перетасовывается.
func TestSurveyOrderIsStable(t *testing.T) {
	g := renderGame(t)
	first := Render{}.Survey(g)
	for i := 0; i < 20; i++ {
		got := Render{}.Survey(g)
		if got != first {
			t.Fatalf("порядок survey поплыл на прогоне %d:\n%q\n%q", i, first, got)
		}
	}
}

// Безопасный переход кости не трогает, и печатать «[d20=0 против 0]» значит
// показывать игроку бросок, которого не было.
func TestNoRollNoRollLine(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.Intent{}, core.TurnResult{Res: &core.Resolution{Class: core.OutcomeSuccess}})
	if strings.Contains(out, "d20") {
		t.Errorf("напечатан несуществующий бросок: %q", out)
	}
}

// Цена провала обязана быть видна в тот же ход. Тик, заметный только когда
// часы заполнятся, — это не цена, а сюрприз через двадцать минут.
func TestCostIsShownTheSameTurn(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.Intent{}, core.TurnResult{
		Res:   &core.Resolution{Class: core.OutcomeFail, Log: core.RollLog{Die: 4}},
		Costs: []core.CostKind{core.CostTickClock, core.CostDispositionDown},
	})
	for _, want := range []string{"часы", "расположение"} {
		if !strings.Contains(out, want) {
			t.Errorf("цена %q не показана: %q", want, out)
		}
	}
}

func TestSuccessShowsNoCostLine(t *testing.T) {
	g := renderGame(t)
	out := Render{}.Turn(g, core.Intent{}, core.TurnResult{
		Res: &core.Resolution{Class: core.OutcomeSuccess, Log: core.RollLog{Die: 18}},
	})
	if strings.Contains(out, "цена") {
		t.Errorf("у успеха появилась цена: %q", out)
	}
}

// --- проза Мастера ---

type fakeNarrator struct {
	prose    string
	err      error
	calls    int
	frames   []string
	outcome  []string
	kinds    []ProseKind
	speaking []string
}

func (n *fakeNarrator) Narrate(_ context.Context, p Prose) (string, error) {
	n.calls++
	n.kinds = append(n.kinds, p.Kind)
	n.frames = append(n.frames, p.Frame)
	n.outcome = append(n.outcome, p.Outcome...)
	n.speaking = append(n.speaking, p.Speaking)
	return n.prose, n.err
}

// Сцену описывает Мастер, а не статичный флейвор. Структура остаётся кодовой:
// заголовок, цели, выходы печатает презентация.
func TestMasterProseReplacesFlavourInScene(t *testing.T) {
	g := renderGame(t)
	n := &fakeNarrator{prose: "Дождь не унимается, доски скользят."}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithNarrator(n)

	got := s.r.Scene(g)
	if !strings.Contains(got, "Дождь не унимается") {
		t.Errorf("проза Мастера не попала в сцену: %q", got)
	}
	if !strings.Contains(got, "==") {
		t.Errorf("структура сцены потерялась: %q", got)
	}
	if n.calls != 1 || !strings.Contains(n.frames[0], g.Flavour("look."+string(g.Node))) {
		t.Errorf("авторская рамка не доехала до Мастера: %d %v", n.calls, n.frames)
	}
}

// Механические строки Мастеру не принадлежат: бросок, «узнали» и цену
// печатает код, иначе проза начнёт врать о механике.
func TestMechanicalLinesStayWithTheCode(t *testing.T) {
	g := renderGame(t)
	n := &fakeNarrator{prose: "Вы всматриваетесь в темноту склада."}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithNarrator(n)

	res := g.Apply(core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}})
	got := s.r.Turn(g, core.Intent{}, res)
	if !strings.Contains(got, "Вы всматриваетесь") {
		t.Errorf("проза Мастера не попала в исход: %q", got)
	}
	if len(res.Learned) > 0 && !strings.Contains(got, "+ узнали") {
		t.Errorf("механическая строка исчезла: %q", got)
	}
	// Исход обязан доехать до Мастера: он описывает то, что произошло.
	if n.calls != 1 {
		t.Fatalf("Мастер вызван %d раз", n.calls)
	}
}

// Сбой Мастера не рушит ход: печатается авторский текст, как без -nl.
func TestNarratorFailureFallsBackToAuthoredProse(t *testing.T) {
	g := renderGame(t)
	n := &fakeNarrator{err: errors.New("шлюз закрыт")}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithNarrator(n)

	got := s.r.Scene(g)
	if !strings.Contains(got, g.Flavour("look."+string(g.Node))) {
		t.Errorf("авторская проза не подставилась после сбоя: %q", got)
	}
}

// Без Мастера всё как раньше: печатается авторский текст.
func TestWithoutNarratorAuthoredProseIsPrinted(t *testing.T) {
	g := renderGame(t)
	s := NewSession(g, strings.NewReader(""), &strings.Builder{})
	if got := s.r.Scene(g); !strings.Contains(got, g.Flavour("look."+string(g.Node))) {
		t.Errorf("авторская проза не напечатана: %q", got)
	}
}

// Молчащий сбой надстройки — худший вид сбоя: игрок видит просто бледный
// текст, а разработчик — ничего. Именно так проза Мастера не работала целую
// фазу при написанном коде и зелёных тестах.
func TestNarratorFailureIsReported(t *testing.T) {
	g := renderGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader(""), &out).
		WithNarrator(&fakeNarrator{err: errors.New("роль не зароутена")})

	fmt.Fprint(&out, s.r.Scene(g))
	if !strings.Contains(out.String(), "роль не зароутена") {
		t.Errorf("сбой Мастера не назван игроку:\n%s", out.String())
	}
	// Названо один раз, а не на каждую строку прозы: шум хуже тишины.
	if n := strings.Count(out.String(), "роль не зароутена"); n != 1 {
		t.Errorf("сбой назван %d раз", n)
	}
}

// Сцена и исход — разные задачи Мастера, и различает их код, а не догадка по
// пустому исходу: у социального хода механики нет вовсе, и «поздороваться»
// описывалось как «игрок озирается по сторонам».
func TestProseKindTellsPlaceFromOutcome(t *testing.T) {
	g := renderGame(t)
	n := &fakeNarrator{prose: "проза"}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithNarrator(n)

	s.r.Scene(g)
	talk := core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_toke"}}
	s.r.Turn(g, talk, g.Apply(talk))

	if len(n.kinds) != 2 {
		t.Fatalf("вызовов прозы %d", len(n.kinds))
	}
	if n.kinds[0] != ProsePlace {
		t.Errorf("сцена описана как %q", n.kinds[0])
	}
	if n.kinds[1] != ProseOutcome {
		t.Errorf("социальный ход описан как %q — механики у него нет, но это исход", n.kinds[1])
	}
}

// Проза исхода печатается ПЕРЕД репликой персонажа, поэтому Мастер обязан
// знать, кто сейчас ответит: иначе он договаривает за него и противоречит.
func TestSpeakerReachesTheNarrator(t *testing.T) {
	g := renderGame(t)
	n := &fakeNarrator{prose: "проза"}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithNarrator(n)

	talk := core.Intent{Verb: "talk_to", Args: core.Args{Target: "e_toke"}}
	s.r.Turn(g, talk, g.Apply(talk))
	if len(n.speaking) != 1 || n.speaking[0] == "" {
		t.Fatalf("говорящий не доехал до Мастера: %v", n.speaking)
	}
	if !strings.Contains(n.speaking[0], "Токе") {
		t.Errorf("говорящим назван %q", n.speaking[0])
	}

	// Осмотр предмета никому слова не даёт: запрет там был бы шумом.
	look := core.Intent{Verb: "examine", Args: core.Args{Target: "e_body"}}
	s.r.Turn(g, look, g.Apply(look))
	if got := n.speaking[1]; got != "" {
		t.Errorf("говорящим назван %q там, где отвечать некому", got)
	}
}

// Игрок обязан видеть, что несёт: предъявлять придётся по имени, и угадывать
// содержимое карманов — не игра.
func TestItemsListsWhatYouCarry(t *testing.T) {
	g := renderGame(t)
	g.DB.Items["i_writ"] = store.Item{ID: "i_writ", Kind: "credential",
		Name: "Предписание магистрата", Text: "Лист с печатью магистрата."}
	g.Acquire("i_writ")

	got := Render{}.Items(g)
	if !strings.Contains(got, "i_writ") || !strings.Contains(got, "Предписание магистрата") {
		t.Errorf("список не назвал предмет:\n%s", got)
	}
}

// Пустые карманы — не ошибка и не пустой экран.
func TestEmptyItemsSaysSo(t *testing.T) {
	if got := (Render{}).Items(renderGame(t)); strings.TrimSpace(got) == "" {
		t.Error("на пустой инвентарь напечатано ничего")
	}
}

// Команда есть в справке: иначе о ней узнают из исходников.
func TestHelpMentionsItems(t *testing.T) {
	if !strings.Contains(Render{}.Help(), "items") {
		t.Error("в справке нет команды items")
	}
}
