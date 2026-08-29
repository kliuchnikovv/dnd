package master

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/llm"
)

func drainStream(t *testing.T, s llm.Stream) (string, int) {
	t.Helper()
	var b strings.Builder
	n := 0
	for {
		d, err := s.Recv()
		b.WriteString(d.Text)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		n++
	}
	return b.String(), n
}

// Неохраняемая проза (пустой state) идёт настоящим стримом: дельты складываются
// в текст Мастера.
func TestNarrateStreamUnguarded(t *testing.T) {
	m, _ := masterWith(t, "Причал тонет в тумане, доски скрипят под ногой")
	st, err := m.NarrateStream(context.Background(), KindPlace, "рамка автора",
		World{Setting: "устье", Scene: []string{"Место: Пристань"}}, nil, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, n := drainStream(t, st)
	if n < 2 {
		t.Fatalf("ждали несколько дельт, пришло %d", n)
	}
	if !strings.Contains(text, "туман") {
		t.Fatalf("проза потерялась: %q", text)
	}
}

// Охраняемая проза (state задан, гвард пропускает) отдаётся косметической
// нарезкой уже проверенного текста.
func TestNarrateStreamGuardedPasses(t *testing.T) {
	m, _ := masterWith(t, "Замок поддаётся, дверь открывается внутрь")
	m = m.WithGuard(&stubChecker{verdicts: []bool{true}})

	st, err := m.NarrateStream(context.Background(), KindOutcome, "рамка",
		World{Setting: "устье", Scene: []string{"Место: Кузница"}},
		[]string{"узнали: замок вскрыт"}, "", []string{"дайджест состояния"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drainStream(t, st)
	if !strings.Contains(text, "дверь открывается") {
		t.Fatalf("проверенный текст не дошёл: %q", text)
	}
}

// Утечку гвард рубит: при провале проверки без ремонта проза схлопывается в
// авторскую рамку, а не отдаёт непроверенный текст.
func TestNarrateStreamGuardedLeakFallsBackToFrame(t *testing.T) {
	m, _ := masterWith(t, "тут модель называет улику — утечка")
	// Оба вердикта — провал: и исходный, и ремонт. Итог Narrate — пусто,
	// стрим отдаёт рамку.
	m = m.WithGuard(&stubChecker{verdicts: []bool{false, false}})

	st, err := m.NarrateStream(context.Background(), KindOutcome, "авторская рамка исхода",
		World{Setting: "устье", Scene: []string{"Место: Кузница"}},
		[]string{"узнали: X"}, "", []string{"дайджест"}, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drainStream(t, st)
	if strings.Contains(text, "утечка") {
		t.Fatalf("непроверенный текст утёк в стрим: %q", text)
	}
	if !strings.Contains(text, "авторская рамка") {
		t.Fatalf("ждали падение на авторскую рамку, получили %q", text)
	}
}

// Пустая рамка — молчать нельзя: стрим отдаёт пусто, но без паники.
func TestNarrateStreamEmptyFrame(t *testing.T) {
	m, _ := masterWith(t, "неважно")
	st, err := m.NarrateStream(context.Background(), KindPlace, "   ",
		World{}, nil, "", nil, llm.Request{})
	if err != nil {
		t.Fatal(err)
	}
	text, _ := drainStream(t, st)
	if text != "" {
		t.Fatalf("пустая рамка дала текст %q", text)
	}
}
