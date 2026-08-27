package cli

import (
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
