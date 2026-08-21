package tui

import (
	"fmt"
	"strings"
	"testing"
)

// Дамп обмена за сессию — мегабайты. Буфер ограничен, но вытесненное
// считается: молча обрезанный лог выглядит как пропавшие вызовы.
func TestRingKeepsLastRecordsAndCountsDropped(t *testing.T) {
	r := NewRing(3)
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(r, "запись %d\n", i)
	}

	text := r.Text()
	if strings.Contains(text, "запись 1") || strings.Contains(text, "запись 2") {
		t.Errorf("старое не вытеснено:\n%s", text)
	}
	if !strings.Contains(text, "запись 5") {
		t.Errorf("свежее потеряно:\n%s", text)
	}
	if got := r.Dropped(); got != 2 {
		t.Errorf("вытеснено %d записей, ждали 2", got)
	}
}

// Пустой буфер — не ошибка: с -debug-llm без вызовов модели показывать нечего.
func TestEmptyRingIsEmpty(t *testing.T) {
	r := NewRing(2)
	if r.Text() != "" || r.Dropped() != 0 {
		t.Errorf("пустой буфер не пуст: %q, вытеснено %d", r.Text(), r.Dropped())
	}
}
