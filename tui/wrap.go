package tui

import "strings"

// wrap ломает текст по ширине, не разрывая слов. Возвращает готовые строки.
//
// Без него длинная строка не переносится, а обрезается по краю окна: проза
// Мастера идёт абзацами в несколько сотен знаков, и игрок читает «рядом с ним
// ж» вместо хода. Прокрутка от этого не спасает — обрезано по горизонтали, а
// не по вертикали.
//
// Свои переводы строки — границы, а не пробелы: список целей и справка
// сверстаны построчно, и склеивать их в абзац нельзя.
func wrap(text string, width int) []string {
	// Бесполезная ширина бывает до первого сообщения о размере окна. Показать
	// всё как есть лучше, чем не показать ничего.
	if width < 2 {
		return strings.Split(text, "\n")
	}
	var out []string
	for _, para := range strings.Split(text, "\n") {
		out = append(out, wrapLine(para, width)...)
	}
	return out
}

func wrapLine(line string, width int) []string {
	if len([]rune(line)) <= width {
		return []string{line}
	}
	var out []string
	var cur []rune
	flush := func() {
		out = append(out, string(cur))
		cur = cur[:0]
	}
	for _, word := range strings.Fields(line) {
		w := []rune(word)
		switch {
		case len(cur) == 0 && len(w) > width:
			// Слово шире строки рвём принудительно: иначе оно уедет за край и
			// утащит за собой всю верстку.
			for len(w) > width {
				out = append(out, string(w[:width]))
				w = w[width:]
			}
			cur = append(cur, w...)
		case len(cur) == 0:
			cur = append(cur, w...)
		case len(cur)+1+len(w) <= width:
			cur = append(cur, ' ')
			cur = append(cur, w...)
		default:
			flush()
			cur = append(cur, w...)
		}
	}
	if len(cur) > 0 {
		flush()
	}
	if len(out) == 0 {
		// Строка из одних пробелов: пустоту сохраняем, она может быть отбивкой.
		return []string{line}
	}
	return out
}
