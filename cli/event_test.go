package cli

import (
	"strings"
	"testing"
)

// TextSink печатает событие дословно: построчный вывод обязан остаться
// побайтово тем же, иначе скрипты и тесты начнут расходиться с игрой.
func TestTextSinkPrintsTextVerbatim(t *testing.T) {
	var out strings.Builder
	sink := TextSink{W: &out}
	sink.Emit(Event{Kind: EventSystem, Text: "== Пристань ==\n"})
	sink.Emit(Event{Kind: EventSpeech, Speaker: "Берн", Text: "— Добрый день.\n"})

	want := "== Пристань ==\n— Добрый день.\n"
	if got := out.String(); got != want {
		t.Errorf("напечатано %q, ждали %q", got, want)
	}
}

// Сессия пишет в приёмник, а не в io.Writer: без этого «кто говорит» можно
// узнать только разбором собственного напечатанного текста.
func TestSessionEmitsToSink(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader("quit\n"), &strings.Builder{}).WithSink(rec)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	if len(rec.events) == 0 {
		t.Fatal("сессия не отдала ни одного события")
	}
	if rec.events[0].Kind != EventScene {
		t.Errorf("первое событие %q, ждали сцену", rec.events[0].Kind)
	}
}

type recordSink struct{ events []Event }

func (r *recordSink) Emit(e Event) { r.events = append(r.events, e) }
