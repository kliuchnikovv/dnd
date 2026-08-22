package tui

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/cli"
)

// Проза Мастера идёт абзацами по 300–500 знаков. Без переноса такая строка
// уходит за край экрана и обрезается на середине слова: игрок читает
// «рядом с ним ж» и не может прочитать ход вовсе.
func TestWrapBreaksLongLineOnWordBoundaries(t *testing.T) {
	long := "Косые полосы дождя бьют по разбухшим доскам, и вода стекает в щели, " +
		"где уже стоят лужи с плёнкой смолы."
	got := wrap(long, 30)

	if len(got) < 3 {
		t.Fatalf("строка длиной %d рун уложилась в %d строк по 30", len([]rune(long)), len(got))
	}
	for i, line := range got {
		if n := len([]rune(line)); n > 30 {
			t.Errorf("строка %d длиной %d рун шире 30: %q", i, n, line)
		}
	}
	// Слова не должны рваться: склейка через пробел обязана дать исходный текст.
	if joined := strings.Join(got, " "); joined != long {
		t.Errorf("перенос порвал текст:\n%q\n%q", joined, long)
	}
}

// Свои переводы строки — границы, а не пробелы: список целей и справка сверстаны
// построчно, и склеивать их в абзац нельзя.
func TestWrapKeepsExistingNewlines(t *testing.T) {
	got := wrap("первая\nвторая", 40)
	if len(got) != 2 || got[0] != "первая" || got[1] != "вторая" {
		t.Errorf("свои переводы строки не сохранены: %q", got)
	}
}

// Слово длиннее строки рвём принудительно: иначе оно уедет за край и утащит
// за собой всю верстку.
func TestWrapHardBreaksOverlongWord(t *testing.T) {
	got := wrap(strings.Repeat("ы", 25), 10)
	if len(got) != 3 {
		t.Fatalf("длинное слово уложено в %d строк: %q", len(got), got)
	}
	for _, line := range got {
		if n := len([]rune(line)); n > 10 {
			t.Errorf("строка шире предела: %d рун", n)
		}
	}
}

// Нулевая и отрицательная ширина не должны съедать текст: до первого
// сообщения о размере окна лучше показать всё как есть, чем ничего.
func TestWrapSurvivesUselessWidth(t *testing.T) {
	for _, w := range []int{0, -5, 1} {
		if got := wrap("что-то сказано", w); len(got) == 0 {
			t.Errorf("при ширине %d текст исчез", w)
		}
	}
}

// Транскрипт переносит прозу под ширину экрана вместе с отступом блока:
// именно здесь обрезка и была видна игроку.
func TestTranscriptWrapsToWidth(t *testing.T) {
	var tr Transcript
	tr.Append(cli.Event{Kind: cli.EventProse,
		Text: "Косые полосы дождя бьют по разбухшим доскам, и вода стекает в щели, " +
			"где уже стоят лужи с плёнкой смолы, а фонарь скрипит на ветру.\n"})

	for _, line := range strings.Split(tr.Render(40), "\n") {
		if n := len([]rune(line)); n > 40 {
			t.Errorf("строка транскрипта шире экрана: %d рун — %q", n, line)
		}
	}
}
