// Package tui — полноэкранный консольный режим. Лежит НАД cli и необязателен:
// без него игра работает построчно, как раньше.
package tui

import "strings"

// History — кольцо введённых фраз. Своё, потому что у поля ввода библиотеки
// истории нет, а разговор на сорок ходов без повтора прошлой фразы не ведут.
type History struct {
	items []string
	// pos — куда смотрит игрок. Равен len(items), когда он не листает.
	pos int
}

// Add кладёт фразу и возвращает курсор в конец: после отправки стрелка вверх
// обязана дать последнее сказанное, а не продолжить прошлую прогулку.
func (h *History) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		h.pos = len(h.items)
		return
	}
	if n := len(h.items); n > 0 && h.items[n-1] == line {
		h.pos = len(h.items)
		return
	}
	h.items = append(h.items, line)
	h.pos = len(h.items)
}

// Prev — шаг назад по истории. false означает, что дальше некуда.
func (h *History) Prev() (string, bool) {
	if h.pos == 0 {
		return "", false
	}
	h.pos--
	return h.items[h.pos], true
}

// Next — шаг вперёд. false на выходе за конец: там пустая строка ввода.
func (h *History) Next() (string, bool) {
	if h.pos+1 >= len(h.items) {
		h.pos = len(h.items)
		return "", false
	}
	h.pos++
	return h.items[h.pos], true
}
