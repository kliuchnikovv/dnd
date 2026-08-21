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
	m.input.SetValue("осмотреться")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.input.Value() != "" {
		t.Errorf("строка ввода не очищена: %q", m.input.Value())
	}
	if got, ok := m.history.Prev(); !ok || got != "осмотреться" {
		t.Errorf("ввод не попал в историю: %q (%v)", got, ok)
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
