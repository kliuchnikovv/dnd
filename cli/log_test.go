package cli

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// Все пять точек испускания идут через одну воронку. Тест на текст, а не на
// поведение, сознательно: прямой s.sink.Emit мимо воронки не ломает ничего
// видимого — он просто тихо выпадает из записи игры, и заметить это можно
// только читая запись, в которой чего-то нет.
func TestEveryEmissionGoesThroughOneFunnel(t *testing.T) {
	body, err := os.ReadFile("repl.go")
	if err != nil {
		t.Fatal(err)
	}
	// Единственное законное упоминание — внутри самой воронки.
	if n := strings.Count(string(body), "s.sink.Emit("); n != 1 {
		t.Errorf("s.sink.Emit зовётся %d раз — часть вывода минует запись игры", n)
	}
}

// Запись отвечает на один вопрос: что игрок писал и что ему отвечали. Значит в
// ней обязано быть и то и другое.
func TestLogHoldsBothInputAndAnswers(t *testing.T) {
	g := renderGame(t)
	var file, screen strings.Builder
	s := NewSession(g, strings.NewReader("survey\nquit\n"), &screen).
		WithLog(NewGameLog(&file, "== тест =="))
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	got := file.String()
	if !strings.Contains(got, "Вы: survey") {
		t.Errorf("ввода игрока в записи нет:\n%s", got)
	}
	if !strings.Contains(got, "Пристань") {
		t.Errorf("ответа игре в записи нет:\n%s", got)
	}
	if !strings.Contains(got, "== тест ==") {
		t.Errorf("заголовка в записи нет:\n%s", got)
	}
}

// Экран от записи не меняется ни на байт. Это главный риск задачи: ввод,
// записанный СОБЫТИЕМ, напечатался бы на экране вторым эхом.
func TestLogDoesNotTouchTheScreen(t *testing.T) {
	script := "survey\nexamine e_body\nquit\n"

	var plain strings.Builder
	if err := NewSession(renderGame(t), strings.NewReader(script), &plain).Run(); err != nil {
		t.Fatal(err)
	}

	var logged, file strings.Builder
	s := NewSession(renderGame(t), strings.NewReader(script), &logged).
		WithLog(NewGameLog(&file, "== тест =="))
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}

	if plain.String() != logged.String() {
		t.Errorf("запись изменила вывод на экран:\nбез записи:\n%s\nс записью:\n%s",
			plain.String(), logged.String())
	}
}

// Ходы пронумерованы: по номеру запись сходится с журналом команд, и разбор
// идёт от читаемой строки к машинной.
func TestLogNumbersTurns(t *testing.T) {
	g := renderGame(t)
	var file strings.Builder
	s := NewSession(g, strings.NewReader("survey\nsurvey\nquit\n"), &strings.Builder{}).
		WithLog(NewGameLog(&file, ""))
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[ход 1]", "[ход 2]"} {
		if !strings.Contains(file.String(), want) {
			t.Errorf("в записи нет %q:\n%s", want, file.String())
		}
	}
}

// Подпись ставится при смене автора: подпись над каждой строкой одного и того
// же говорящего — шум.
func TestLogSignsOnlyOnSpeakerChange(t *testing.T) {
	var file strings.Builder
	l := NewGameLog(&file, "")
	l.event(Event{Kind: EventProse, Text: "первая\n"})
	l.event(Event{Kind: EventProse, Text: "вторая\n"})
	if n := strings.Count(file.String(), MasterName+":"); n != 1 {
		t.Errorf("подпись Мастера стоит %d раз на двух его строках подряд:\n%s",
			n, file.String())
	}
}

// Набор вариантов в запись не идёт: это панель, которая себя заменяет каждый
// ход, и в потоке она копилась бы дюжинами устаревших копий.
func TestLogSkipsTheOptionsPanel(t *testing.T) {
	var file strings.Builder
	l := NewGameLog(&file, "")
	l.event(Event{Kind: EventOptions, Text: "Что можно:\n  1. осмотреть\n"})
	if strings.Contains(file.String(), "Что можно") {
		t.Errorf("панель вариантов попала в запись:\n%s", file.String())
	}
}

// Поломка записи ход не рушит и не молчит о себе. Игра, падающая из-за
// надстройки, хуже игры без надстройки; игра, молча пишущая в никуда, хуже обеих.
func TestBrokenLogDoesNotKillTheRun(t *testing.T) {
	g := renderGame(t)
	var screen strings.Builder
	s := NewSession(g, strings.NewReader("survey\nquit\n"), &screen).
		WithLog(NewGameLog(failWriter{}, ""))
	if err := s.Run(); err != nil {
		t.Fatalf("поломка записи уронила прогон: %v", err)
	}
	if !strings.Contains(screen.String(), "Пристань") {
		t.Errorf("поломка записи съела вывод игры:\n%s", screen.String())
	}
	if !strings.Contains(screen.String(), "запись игры") {
		t.Errorf("о поломке записи не сказано:\n%s", screen.String())
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("диск полон") }

// Без флага запись не ведётся вовсе, и nil-лог не падает: игра без надстройки
// обязана работать как работала.
func TestNilLogIsSilent(t *testing.T) {
	var l *GameLog
	l.event(Event{Kind: EventProse, Text: "проза\n"})
	l.turn(1)
	l.input("осмотреться")
}
