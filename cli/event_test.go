package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
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

// Метка говорящего берётся из данных, а не из тире в начале строки: разбирать
// собственный вывод, чтобы понять, кто сказал, — та же ошибка, что читать
// канон из прозы.
func TestSpeechCarriesSpeaker(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	fv := &fakeVoicer{line: "Мокро сегодня."}
	s := NewSession(g, strings.NewReader("talk_to toke\nquit\n"), &strings.Builder{}).
		WithSink(rec).WithVoicer(fv)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}

	var speech []Event
	for _, e := range rec.events {
		if e.Kind == EventSpeech {
			speech = append(speech, e)
		}
	}
	if len(speech) != 1 {
		t.Fatalf("событий речи %d, ждали одно", len(speech))
	}
	if !strings.Contains(speech[0].Speaker, "Токе") {
		t.Errorf("говорящий %q", speech[0].Speaker)
	}
	if !strings.Contains(speech[0].Text, "Мокро сегодня.") {
		t.Errorf("реплика %q", speech[0].Text)
	}
}

// Речь игрока — тоже речь, и у неё тоже есть автор.
func TestPlayerSpeechIsMarkedAsPlayers(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	fi := &fakeInterp{intent: &core.Intent{Verb: "say",
		Args: core.Args{Target: "e_toke", Text: "здравствуйте"}}}
	s := NewSession(g, strings.NewReader("здравствуйте\nquit\n"), &strings.Builder{}).
		WithSink(rec).WithInterpreter(fi)
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for _, e := range rec.events {
		if e.Kind == EventSpeech && e.Speaker == PlayerName {
			return
		}
	}
	t.Errorf("речь игрока не помечена автором: %+v", rec.events)
}
