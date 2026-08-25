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
	// Порядок старта: брифинг (проза Мастера), известное (печать кода), место.
	// Игрок обязан узнать, зачем он здесь, прежде чем увидеть, где он.
	var order []EventKind
	for _, e := range rec.events {
		order = append(order, e.Kind)
	}
	want := []EventKind{EventProse, EventSystem, EventScene}
	for i, k := range want {
		if i >= len(order) || order[i] != k {
			t.Fatalf("порядок старта %v, ждали %v", order, want)
		}
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

// Отказ мира произносит Мастер: в чате безличная служебная строка рядом с
// живыми репликами читается как поломка, а не как ответ собеседника.
func TestRefusalCarriesMasterAsSpeaker(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader("question toke f_нет\nquit\n"), &strings.Builder{}).
		WithSink(rec).
		WithRefusalVoice(func(string) string { return "Токе отводит взгляд: об этом не здесь." })
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}

	for _, e := range rec.events {
		if e.Kind != EventRefusal {
			continue
		}
		if e.Speaker != MasterName {
			t.Errorf("отказ без автора: %+v", e)
		}
		if !strings.Contains(e.Text, "отводит взгляд") {
			t.Errorf("голос Мастера не доехал: %q", e.Text)
		}
		// Механику печатает код, а не модель: «ход не потрачен» — это факт
		// движка, и отдавать его прозе значит позволить ей об этом врать.
		if !strings.Contains(e.Text, "ход не потрачен") {
			t.Errorf("потерян признак того, что отказ не потратил ход: %q", e.Text)
		}
		return
	}
	t.Fatalf("отказа в событиях нет: %+v", rec.events)
}

// Без голоса Мастера отказ печатается как раньше, побайтово: игра без моделей
// обязана работать как работала.
func TestRefusalWithoutVoiceIsUnchanged(t *testing.T) {
	var plain, voiced strings.Builder
	script := "question toke f_нет\nquit\n"
	if err := NewSession(renderGame(t), strings.NewReader(script), &plain).Run(); err != nil {
		t.Fatal(err)
	}
	// Голос, который промолчал (сбой модели), обязан дать тот же вывод: откат
	// на авторский текст — не «почти как раньше», а ровно как раньше.
	if err := NewSession(renderGame(t), strings.NewReader(script), &voiced).
		WithRefusalVoice(func(string) string { return "" }).Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.String(), "нельзя:") {
		t.Fatalf("отказ не напечатан: %q", plain.String())
	}
	if plain.String() != voiced.String() {
		t.Errorf("молчащий голос изменил вывод:\n без: %q\n с:   %q", plain.String(), voiced.String())
	}
}

// Подсказке не нужен говорящий: она принадлежит игроку, а не персонажу.
// Раньше она уходила речью напарника и требовала, чтобы тот был в деле; теперь
// требуется только сама подсказка.
func TestHunchIsEmittedWithoutAnySpeakingEntity(t *testing.T) {
	// Дело «Гавань»: в минимальном единственная подсказка указывает на факт из
	// start_facts, то есть на уже известное, и сработать не может никогда.
	g := harbourGame(t)
	rec := &recordSink{}
	// Отказы подряд — самый чистый признак того, что игрок встал.
	script := strings.Repeat("question bern f_нет\n", core.HintAfter) + "quit\n"
	// Чутьё выключено по умолчанию — тест проверяет его саму механику,
	// поэтому включает.
	s := NewSession(g, strings.NewReader(script), &strings.Builder{}).WithSink(rec).WithHunch()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}

	for _, e := range rec.events {
		if e.Kind != EventHunch {
			continue
		}
		if e.Speaker != HunchName {
			t.Errorf("подсказка подписана %q, ждали %q", e.Speaker, HunchName)
		}
		return
	}
	t.Fatalf("подсказки в событиях нет: %+v", rec.events)
}

// По умолчанию чутьё молчит. Живой прогон показал, почему: счётчик холостых
// ходов считает любой ход без находки, а осмотр без находки — это нормальный
// осмотр, а не «встал». Подсказка приходила спокойно исследующему игроку и
// читала решение вслух; не вовремя она хуже, чем никак.
func TestHunchIsSilentUnlessAskedFor(t *testing.T) {
	g := harbourGame(t)
	rec := &recordSink{}
	script := strings.Repeat("question bern f_нет\n", core.HintAfter+2) + "quit\n"
	if err := NewSession(g, strings.NewReader(script), &strings.Builder{}).
		WithSink(rec).Run(); err != nil {
		t.Fatal(err)
	}
	for _, e := range rec.events {
		if e.Kind == EventHunch {
			t.Errorf("чутьё сработало без спроса: %q", e.Text)
		}
	}
}
