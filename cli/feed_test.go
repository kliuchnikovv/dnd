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
