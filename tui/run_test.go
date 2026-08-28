package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
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

// Стартовый экран полноэкранного режима несёт то же, что построчный: брифинг,
// известное, сцену и список вариантов. Полноэкранный режим — второй ПРИЁМНИК
// одного вывода, а не второй его формат, и молча потерять здесь блок значило бы
// развести режимы.
func TestStartReachesTheFullscreenSink(t *testing.T) {
	cfg, err := cases.Load("../cases/harbour/case.json")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Rules = threshold.New()
	cfg.Dice = dice.NewSource(1).Stream("resolve")
	g := core.NewGame(*cfg)

	m := newModel(nil, Options{})
	s := cli.NewSession(g, strings.NewReader(""), io.Discard).WithAffordances()
	s.WithSink(sink{ch: m.events, done: m.done})
	s.Start()

	var got strings.Builder
	for len(m.events) > 0 {
		got.WriteString((<-m.events).Text)
	}
	for _, want := range []string{"Что известно:", "== Пристань ==", "Что можно:", "своими словами"} {
		if !strings.Contains(got.String(), want) {
			t.Errorf("на стартовом экране нет %q:\n%s", want, got.String())
		}
	}
}
