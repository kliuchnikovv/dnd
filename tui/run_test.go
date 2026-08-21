package tui

import (
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/cli"
)

// После Ctrl-C программа больше не читает events, а горутина хода может
// ещё дописывать в sink. Без done.Emit блокируется на переполненном канале
// навечно — сюда и целится тест: канал без буфера, читателя нет, done уже
// закрыт (интерфейс уже вышел). Разумный таймаут вместо вечного ожидания —
// единственный способ отличить «работает» от «висит до убийства процесса».
func TestSinkEmitDoesNotBlockAfterInterfaceClosed(t *testing.T) {
	done := make(chan struct{})
	close(done) // интерфейс уже закрылся

	s := sink{ch: make(chan cli.Event), done: done} // без буфера и без читателя

	finished := make(chan struct{})
	go func() {
		s.Emit(cli.Event{Kind: cli.EventProse, Text: "ход всё ещё пишет"})
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("Emit завис после закрытия интерфейса")
	}
}
