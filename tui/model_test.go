package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// Ввод уходит в игру по Enter и попадает в историю: без этого стрелка вверх
// пуста, а ради неё всё и затевалось.
func TestEnterSendsInputAndRemembersIt(t *testing.T) {
	m := newModel(nil, Options{})
	// Стартовая сцена «допечатана» — иначе Enter заблокирован тем же busy,
	// что и ход (см. TestEnterIsBlockedUntilStartFinishes ниже).
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	m.input.SetValue("осмотреться")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "" {
		t.Errorf("строка ввода не очищена: %q", m.input.Value())
	}
	if got, ok := m.history.Prev(); !ok || got != "осмотреться" {
		t.Errorf("ввод не попал в историю: %q (%v)", got, ok)
	}
}

// Enter, нажатый раньше, чем s.Start() допечатал стартовую сцену, не должен
// проходить: иначе s.Feed запустится параллельно с ещё живым s.Start, а
// cli.Session рассчитан только на строго последовательные вызовы.
func TestEnterIsBlockedUntilStartFinishes(t *testing.T) {
	m := newModel(nil, Options{})
	m.input.SetValue("рано")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "рано" {
		t.Error("ввод принят до конца стартовой сцены")
	}
}

// startedMsg — единственный способ снять стартовую блокировку; без него
// стрелка вверх и Enter молчат вечно.
func TestStartedMsgUnblocksInput(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(startedMsg{})
	m = next.(model)

	if m.busy {
		t.Error("busy не снят после отметки о завершении старта")
	}
}

// Ширина по умолчанию ненулевая: до первого tea.WindowSizeMsg транскрипт
// всё равно должен рендериться в разумную колонку, а не в нулевую.
func TestDefaultWidthIsNotZero(t *testing.T) {
	m := newModel(nil, Options{})
	if m.width <= 0 {
		t.Errorf("ширина по умолчанию не задана: %d", m.width)
	}
}

// Пока ход идёт, ввод заблокирован: два хода одновременно ядро не переживёт.
func TestInputIsBlockedWhileTurnRuns(t *testing.T) {
	m := newModel(nil, Options{})
	m.busy = true
	m.input.SetValue("второй ход")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "второй ход" {
		t.Error("ввод принят во время хода")
	}
}

// События игры попадают в транскрипт с автором.
func TestEventGoesToTranscript(t *testing.T) {
	m := newModel(nil, Options{})
	next, _ := m.Update(eventMsg{cli.Event{
		Kind: cli.EventSpeech, Speaker: "Берн", Text: "— Добрый день.\n"}})
	m = next.(model)

	if !strings.Contains(m.transcript.Render(60), "Берн") {
		t.Error("событие не доехало до транскрипта")
	}
}

// Tab переключает панель отладки и обратно.
func TestTabTogglesDebugPane(t *testing.T) {
	m := newModel(nil, Options{Debug: NewRing(4)})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if !next.(model).debugOpen {
		t.Fatal("Tab не открыл панель отладки")
	}
	next, _ = next.(model).Update(tea.KeyMsg{Type: tea.KeyTab})
	if next.(model).debugOpen {
		t.Error("Tab не закрыл панель отладки")
	}
}
