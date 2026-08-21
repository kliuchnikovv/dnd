package cli

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Обвинение читало четыре строки вложенным циклом. Полноэкранный режим отдаёт
// ввод по одной строке и заблокироваться не может, поэтому набор слотов —
// состояние сессии, а не вложенный цикл.
func TestAccusationFillsSlotsOneFeedAtATime(t *testing.T) {
	g := renderGame(t)
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithSink(rec)

	s.startAccusation()
	if !s.awaitingAccusation() {
		t.Fatal("после начала обвинения сессия не ждёт токен")
	}
	for i, tok := range []string{"toke", "cord", "night", "audit"} {
		if !s.awaitingAccusation() {
			t.Fatalf("на слоте %d сессия перестала ждать ввод", i)
		}
		s.feedAccusation(tok)
	}
	if s.awaitingAccusation() {
		t.Error("после четырёх токенов обвинение всё ещё ждёт ввод")
	}
	if !s.Game.Solved() {
		t.Error("верное обвинение не закрыло дело")
	}
}

// Пустой слот прекращает набор, а не вешает сессию в ожидании ввода, которого
// игрок дать не может.
func TestEmptySlotEndsAccusation(t *testing.T) {
	// Игра без токенов вовсе: фикстура minimal.json отдаёт слот who сразу, и
	// на ней пустой слот не воспроизвести.
	g := core.NewGame(core.Config{DB: store.NewDB()})
	rec := &recordSink{}
	s := NewSession(g, strings.NewReader(""), &strings.Builder{}).WithSink(rec)
	s.startAccusation()
	if s.awaitingAccusation() {
		t.Error("сессия ждёт токен, которого взять негде")
	}
	var said string
	for _, e := range rec.events {
		said += e.Text
	}
	if !strings.Contains(said, "пуст") {
		t.Errorf("игроку не сказано, что слот пуст: %q", said)
	}
}
