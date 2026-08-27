package tui

import (
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

// optionsModel — модель с уже показанным набором: сессия отработала Start, и
// панель забрала варианты тем же путём, каким их забирает живой прогон.
func optionsModel(t *testing.T) model {
	t.Helper()
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	g := core.NewGame(*cfg)

	m := newModel(cli.NewSession(g, strings.NewReader(""), io.Discard).WithAffordances(), Options{})
	m.session.WithSink(sink{ch: m.events, done: m.done})
	m.session.Start()
	m.busy = false
	for len(m.events) > 0 {
		e := <-m.events
		if e.Kind == cli.EventOptions {
			m.takeOptions()
		}
	}
	return m
}

// Набор живёт панелью у ввода, а не строками транскрипта: он себя заменяет
// каждый ход, и в потоке копился бы устаревшими копиями.
func TestOptionsLiveInAPaneNotTheTranscript(t *testing.T) {
	m := optionsModel(t)
	if len(m.offered) == 0 {
		t.Fatal("панель не забрала варианты")
	}
	if got := m.transcript.Render(80); strings.Contains(got, "Что можно:") {
		t.Errorf("набор попал в транскрипт:\n%s", got)
	}
	if !strings.Contains(m.optionsView(), "Что можно:") {
		t.Errorf("панель пуста:\n%s", m.optionsView())
	}
}

// Панель стоит НИЖЕ транскрипта и выше строки ввода: выбирают её тем же
// движением, каким печатают, и глазу нужно то, что рядом с курсором.
func TestOptionsPaneSitsAboveTheInput(t *testing.T) {
	m := optionsModel(t)
	view := m.View()
	options := strings.Index(view, "Что можно:")
	input := strings.Index(view, m.input.Prompt)
	if options < 0 || input < 0 {
		t.Fatalf("на экране нет панели или ввода:\n%s", view)
	}
	if options > input {
		t.Errorf("панель оказалась ниже ввода:\n%s", view)
	}
}

// Стрелки водят курсор по вариантам. Кольцо через «ничего не выбрано»: у
// списка из четырёх строк это короче любого объяснения.
func TestArrowsWalkTheOptions(t *testing.T) {
	m := optionsModel(t)
	if m.selected != noSelection {
		t.Fatalf("на старте хода что-то подсвечено: %d", m.selected)
	}
	m.selectionDown()
	if m.selected != 0 {
		t.Errorf("вниз с ничего дал %d, ожидался первый", m.selected)
	}
	m.selectionUp()
	if m.selected != noSelection {
		t.Errorf("вверх с первого дал %d, ожидалось ничего", m.selected)
	}
	m.selectionUp()
	if m.selected != len(m.offered)-1 {
		t.Errorf("вверх с ничего дал %d, ожидался последний", m.selected)
	}
}

// Выбор уходит номером — тем же путём применения, что набранная цифра.
// Полноэкранный режим не второй вход со своей правдой о том, что игрок сделал.
func TestSelectionIsFedAsANumber(t *testing.T) {
	m := optionsModel(t)
	m.selectionDown()
	m.selectionDown()
	line, ok := m.chosenLine()
	if !ok || line != "2" {
		t.Errorf("выбор ушёл как %q (ok=%v), ожидался номер варианта", line, ok)
	}
}

// В транскрипте за выбранный вариант печатаются СЛОВА: игрок выбрал ход, а не
// нажал «3». Сырой ввод для аудита при этом остаётся номером.
func TestTranscriptEchoesWordsNotTheNumber(t *testing.T) {
	m := optionsModel(t)
	m.selectionDown()
	echo := m.chosenEcho()
	if echo == "" || echo == "1" {
		t.Errorf("эхо выбора негодно: %q", echo)
	}
	if !strings.Contains(echo, cli.AffordanceLabel(m.session.Game, m.offered[0])) {
		t.Errorf("эхо не совпало с ярлыком варианта: %q", echo)
	}
}

// Набранная фраза старше выбора: игрок, начавший печатать, передумал, и
// исполнить вместо его слов подсвеченную строку значило бы проглотить
// набранное.
func TestTypedTextBeatsTheHighlight(t *testing.T) {
	m := optionsModel(t)
	m.selectionDown()
	m.input.SetValue("осмотреть бочки")
	next, _ := m.key(tea.KeyMsg{Type: tea.KeyEnter})
	got := next.(model)
	if line := got.transcript.Render(80); !strings.Contains(line, "осмотреть бочки") {
		t.Errorf("набранное проглочено подсветкой:\n%s", line)
	}
}

// История переехала на Ctrl-P/Ctrl-N: стрелки заняты вариантами. Варианты
// нужны каждый ход, история — изредка, и частый жест обязан быть проще.
func TestHistoryMovedToCtrlPN(t *testing.T) {
	m := optionsModel(t)
	m.history.Add("осмотреть бочки")

	next, _ := m.key(tea.KeyMsg{Type: tea.KeyCtrlP})
	if got := next.(model).input.Value(); got != "осмотреть бочки" {
		t.Errorf("Ctrl-P не поднял историю: %q", got)
	}
	// Стрелка вверх историю больше не трогает — она водит курсор по списку.
	up, _ := m.key(tea.KeyMsg{Type: tea.KeyUp})
	if got := up.(model); got.input.Value() != "" {
		t.Errorf("стрелка вверх подставила историю: %q", got.input.Value())
	} else if got.selected == noSelection {
		t.Error("стрелка вверх не тронула выбор варианта")
	}
}

// Подвал говорит, что стрелки теперь делают. Раскладка, о которой не сказано,
// не существует для игрока.
func TestFooterNamesTheNewBindings(t *testing.T) {
	view := optionsModel(t).View()
	for _, want := range []string{"↑↓ варианты", "^P/^N история"} {
		if !strings.Contains(view, want) {
			t.Errorf("подвал не называет %q", want)
		}
	}
}

// Панель вариантов берёт реплики в кавычки, а действия — нет: cli.Quoted
// экспортирована ровно затем, чтобы tui не завёл свою копию правила, и это
// обязано быть проверено здесь, а не только в cli — иначе локальная копия в
// tui молча разошлась бы с ней и тест бы этого не заметил.
func TestOptionsPaneQuotesReplies(t *testing.T) {
	m := optionsModel(t)
	m.offered = []core.Affordance{
		{Intent: core.Intent{Verb: "ask_about", Args: core.Args{Target: "e_toke"}}, Reply: true},
		{Intent: core.Intent{Verb: "examine", Args: core.Args{Target: "p_crates"}}},
	}
	m.selected = noSelection
	view := m.optionsView()
	if !strings.Contains(view, cli.Quoted(cli.AffordanceLabel(m.session.Game, m.offered[0]))) {
		t.Errorf("реплика не в кавычках:\n%s", view)
	}
	if strings.Contains(view, "«"+cli.AffordanceLabel(m.session.Game, m.offered[1])+"»") {
		t.Errorf("действие взято в кавычки:\n%s", view)
	}
}

// Транскрипт не уезжает под панель: высота считается по тому, что реально
// напечатано, а не по константе.
func TestPaneDoesNotPushTheTranscriptOffScreen(t *testing.T) {
	m := optionsModel(t)
	m.height = 24
	m.refresh()
	chrome := 3 + lines(m.optionsPane()) + lines(m.promptView())
	if want := 24 - chrome; m.view.Height != want {
		t.Errorf("высота транскрипта %d, а обвязка занимает %d из 24",
			m.view.Height, chrome)
	}
	if m.view.Height < 1 {
		t.Error("транскрипту не осталось места")
	}
}
