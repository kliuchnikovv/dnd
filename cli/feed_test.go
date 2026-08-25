package cli

import (
	"strings"
	"testing"
)

// Полноэкранному режиму нужен ход по одной строке: циклом владеет он, а не
// сессия. Логика при этом та же самая, и оба драйвера обязаны давать
// одинаковый результат.
func TestFeedRunsOneTurn(t *testing.T) {
	g := renderGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader(""), &out)

	s.Start()
	if !strings.Contains(out.String(), "==") {
		t.Error("Start не напечатал стартовую сцену")
	}
	if done := s.Feed("look"); done {
		t.Error("осмотр закончил игру")
	}
	if done := s.Feed("quit"); !done {
		t.Error("quit не закончил игру")
	}
}

// Ход считается один раз на ввод: по нему считается потолок вызовов модели, и
// сбитый счёт означает сбитый лимит.
func TestFeedCountsTurns(t *testing.T) {
	g := renderGame(t)
	s := NewSession(g, strings.NewReader(""), &strings.Builder{})
	s.Start()
	s.Feed("look")
	s.Feed("look")
	if got := s.Turn(); got != 2 {
		t.Errorf("ходов %d, ждали 2", got)
	}
}

// Run — тот же Feed в цикле: построчный режим обязан остаться побайтово тем
// же, иначе скрипты и тесты разойдутся с игрой.
func TestRunAndFeedAgree(t *testing.T) {
	script := "look\nfacts\nquit\n"

	var viaRun strings.Builder
	if err := NewSession(renderGame(t), strings.NewReader(script), &viaRun).Run(); err != nil {
		t.Fatal(err)
	}

	var viaFeed strings.Builder
	s := NewSession(renderGame(t), strings.NewReader(""), &viaFeed)
	s.Start()
	for _, line := range []string{"look", "facts", "quit"} {
		if s.Feed(line) {
			break
		}
	}

	if viaRun.String() != viaFeed.String() {
		t.Errorf("драйверы разошлись:\n--- Run ---\n%s\n--- Feed ---\n%s",
			viaRun.String(), viaFeed.String())
	}
}

// Игра начинается брифингом: кто ты, что случилось, чего от тебя ждут. Живой
// прогон встал ровно на его отсутствии — «мы ничего не знаем о деле в начале
// игры», и всё, что дальше опиралось на склад и на предписание магистрата,
// читалось как знание из ниоткуда. Материал у дела был: стартовый факт и
// предмет в инвентаре. Игра их не показывала.
func TestStartOpensWithABriefing(t *testing.T) {
	g := harbourGame(t)
	var out strings.Builder
	NewSession(g, strings.NewReader(""), &out).Start()

	got := out.String()
	// Авторский текст брифинга.
	if !strings.Contains(got, g.Briefing) || strings.TrimSpace(g.Briefing) == "" {
		t.Errorf("авторский брифинг не напечатан:\n%s", got)
	}
	// Что известно: стартовый факт называется своими словами.
	if !strings.Contains(got, "Халдена") {
		t.Errorf("известное о деле не показано:\n%s", got)
	}
	// Что при себе: без этого «предъявить предписание» неоткуда узнать.
	if !strings.Contains(got, "Предписание магистрата") {
		t.Errorf("инвентарь не показан:\n%s", got)
	}
	// И только потом место.
	if strings.Index(got, "== ") < strings.Index(got, "Халдена") {
		t.Errorf("сцена напечатана раньше брифинга:\n%s", got)
	}
}

// Брифинг печатается ОДИН раз, на старте: повторять его каждый ход — шум, а
// перечитать можно командами facts и items.
func TestBriefingIsPrintedOnce(t *testing.T) {
	g := harbourGame(t)
	var out strings.Builder
	s := NewSession(g, strings.NewReader(""), &out)
	s.Start()
	s.Feed("look")

	if got := strings.Count(out.String(), g.Briefing); got != 1 {
		t.Errorf("брифинг напечатан %d раз, ждали один", got)
	}
}
