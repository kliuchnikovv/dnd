package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kliuchnikovv/dnd/cli"
)

// sink переправляет события игры в интерфейс. Канал, а не прямая запись:
// ход идёт в своей горутине, а трогать модель из неё нельзя.
//
// done — второй исход для Emit. Ctrl-C посреди хода завершает программу, не
// дожидаясь конца s.Feed; после этого events никто больше не читает. Без
// done блокирующая запись в переполненный events держала бы горутину хода
// вечно — до убийства процесса. done закрывается моделью в quit() ровно
// один раз, и Emit просто перестаёт ждать место в канале.
type sink struct {
	ch   chan cli.Event
	done chan struct{}
}

func (s sink) Emit(e cli.Event) {
	select {
	case s.ch <- e:
	case <-s.done:
	}
}

// Run открывает полноэкранный режим и не возвращается до выхода из игры.
func Run(s *cli.Session, opts Options) error {
	m := newModel(s, opts)
	s.WithSink(sink{ch: m.events, done: m.done})
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}
