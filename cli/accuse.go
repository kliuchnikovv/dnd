package cli

import (
	"strings"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// accusation4 — набор четырёх слотов обвинения. Состояние, а не вложенный
// цикл ввода: драйвер отдаёт по одной строке и ждать следующую внутри хода не
// может, а обвинение — единственный способ закончить дело.
type accusation4 struct {
	form accusation.Form
	idx  int
}

var accusationSlots = []struct {
	name string
	dst  func(*accusation.Form) *store.Token
}{
	{"who", func(f *accusation.Form) *store.Token { return &f.Who }},
	{"how", func(f *accusation.Form) *store.Token { return &f.How }},
	{"when", func(f *accusation.Form) *store.Token { return &f.When }},
	{"why", func(f *accusation.Form) *store.Token { return &f.Why }},
}

func (s *Session) awaitingAccusation() bool { return s.accusing != nil }

// startAccusation открывает набор и печатает приглашение первого слота.
func (s *Session) startAccusation() {
	s.accusing = &accusation4{}
	s.promptSlot()
}

// promptSlot печатает приглашение текущего слота либо закрывает набор, если
// слот пуст: подсказывать нечего, а ждать ввод, которого игрок дать не может,
// значит подвесить игру.
func (s *Session) promptSlot() {
	slot := accusationSlots[s.accusing.idx]
	avail := s.Game.AvailableTokens(slot.name)
	if len(avail) == 0 {
		s.emit(EventSystem, "слот %s пуст: нужных фактов ещё нет\n", slot.name)
		s.accusing = nil
		return
	}
	parts := make([]string, len(avail))
	for i, a := range avail {
		parts[i] = string(a)
	}
	s.emit(EventPrompt, "%s: %s\n> ", slot.name, strings.Join(parts, " | "))
}

// feedAccusation заполняет текущий слот и двигает набор дальше.
func (s *Session) feedAccusation(line string) {
	slot := accusationSlots[s.accusing.idx]
	*slot.dst(&s.accusing.form) = store.Token(strings.TrimSpace(line))
	s.accusing.idx++
	if s.accusing.idx < len(accusationSlots) {
		s.promptSlot()
		return
	}
	form := s.accusing.form
	s.accusing = nil
	s.resolveAccusation(form)
}

// resolveAccusation — исход обвинения. Ошибочная форма не сообщает, какой слот
// неверен: иначе слоты брутфорсятся по одному.
func (s *Session) resolveAccusation(form accusation.Form) {
	g := s.Game
	res := g.Accuse(form)
	switch {
	case res.Refused:
		s.emit(EventRefusal, "нельзя: %s\n", res.Refusal)
	case res.Correct:
		s.emit(EventSystem, "Обвинение верно. Попыток: %d.\n\n", res.Attempt)
		for _, clause := range res.Summation {
			// Клаузы саммации — речь игрока: он произносит обвинение вслух.
			// У EventSpeech автор обязателен (спек §3), а TextSink печатает
			// только Text — построчный вывод байт в байт не меняется.
			s.emitSpeech(PlayerName, "  — %s\n", clause)
		}
		if after := g.Aftermath(); after != "" {
			s.emit(EventSystem, "\n%s\n", after)
		}
	default:
		s.emit(EventSystem, "В обвинении есть ошибки. Попыток: %d.\n", res.Attempt)
	}
	for _, c := range res.Fired {
		s.emit(EventSystem, "  ⏱ %s\n", g.Flavour(c.FlavourKey))
	}
}
