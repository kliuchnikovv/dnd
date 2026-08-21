package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// sink переправляет события игры в интерфейс. Канал, а не прямая запись:
// ход идёт в своей горутине, а трогать модель из неё нельзя.
type sink struct{ ch chan cli.Event }

func (s sink) Emit(e cli.Event) { s.ch <- e }

// Run открывает полноэкранный режим и не возвращается до выхода из игры.
func Run(s *cli.Session, opts Options) error {
	m := newModel(s, opts)
	s.WithSink(sink{ch: m.events})
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
