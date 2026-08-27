package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/kliuchnikovv/dnd/cli"
	"github.com/kliuchnikovv/dnd/core"
)

// Панель вариантов: список у самой строки ввода, выбор стрелками.
//
// Панель, а не строки транскрипта, по той же причине, что и приглашение
// (EventPrompt): набор себя ЗАМЕНЯЕТ каждый ход. В общем потоке он копился бы
// дюжинами устаревших копий, и прокрутка к прошлой реплике проходила бы сквозь
// четыре списка, три из которых уже неверны.
//
// Внизу, а не наверху: выбирают его тем же движением, каким печатают, и глазу
// нужно то, что рядом с курсором.

// noSelection — курсор ни на чём. Стартовое состояние каждого хода: выбор
// делается осознанно, а подсвеченная сама собой первая строка означала бы, что
// Enter на пустой строке исполняет ход, которого игрок не выбирал.
const noSelection = -1

// selectionUp двигает курсор к началу списка. С «ничего не выбрано» шаг вверх
// ведёт в конец: у списка из четырёх строк кольцо короче любого объяснения.
func (m *model) selectionUp() {
	if len(m.offered) == 0 {
		return
	}
	switch m.selected {
	case noSelection:
		m.selected = len(m.offered) - 1
	case 0:
		m.selected = noSelection
	default:
		m.selected--
	}
}

func (m *model) selectionDown() {
	if len(m.offered) == 0 {
		return
	}
	switch m.selected {
	case noSelection:
		m.selected = 0
	case len(m.offered) - 1:
		m.selected = noSelection
	default:
		m.selected++
	}
}

// optionsView — панель вариантов под транскриптом. Пустая строка означает, что
// вариантов нет: заголовок без списка читался бы как поломка.
//
// Нумерация остаётся видимой, хотя выбор идёт стрелками: номер работает и
// набранный руками, а построчный режим кроме него ничего и не знает. Показать
// стрелками одно, а цифрой другое — это два разных списка.
func (m model) optionsView() string {
	if len(m.offered) == 0 || m.session == nil {
		return ""
	}
	sel := lipgloss.NewStyle().Bold(true).Reverse(true)
	faint := lipgloss.NewStyle().Faint(true)

	words := m.session.OfferedWords()
	var b strings.Builder
	b.WriteString(faint.Render("Что можно:") + "\n")
	for i, a := range m.offered {
		label := cli.AffordanceLabel(m.session.Game, a)
		if len(words) == len(m.offered) {
			label = words[i]
		}
		line := "  " + strconv.Itoa(i+1) + ". " + label
		if i == m.selected {
			b.WriteString(sel.Render(line) + "\n")
			continue
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(faint.Render("  ↑↓ выбрать · Enter исполнить · или напиши своими словами"))
	return b.String()
}

// chosenLine — ввод, который отправляет выбранный вариант. Номер, а не своя
// команда: и стрелки, и набранная цифра обязаны идти ОДНИМ путём применения
// через Feed, иначе полноэкранный режим стал бы вторым входом со своей правдой
// о том, что игрок сделал.
func (m model) chosenLine() (string, bool) {
	if m.selected < 0 || m.selected >= len(m.offered) {
		return "", false
	}
	return strconv.Itoa(m.selected + 1), true
}

// chosenEcho — что показать в транскрипте за выбранный вариант. Слова, а не
// цифра: игрок выбрал ход, а не нажал «3», и в записи разговора цифра означала
// бы, что он её печатал. Сырой ввод для аудита при этом остаётся номером — это
// разные правды, и они не обязаны совпадать (ADR-0002).
func (m model) chosenEcho() string {
	if m.selected < 0 || m.selected >= len(m.offered) || m.session == nil {
		return ""
	}
	words := m.session.OfferedWords()
	if len(words) == len(m.offered) {
		return words[m.selected]
	}
	return cli.AffordanceLabel(m.session.Game, m.offered[m.selected])
}

// takeOptions забирает набор из сессии. Событие говорит «набор сменился», а
// сам набор берётся структурой: разбирать напечатанные строки значило бы
// парсить собственный вывод.
func (m *model) takeOptions() {
	if m.session == nil {
		m.offered = nil
		m.selected = noSelection
		return
	}
	m.offered = append([]core.Affordance(nil), m.session.Offered()...)
	// Курсор сбрасывается на каждый новый набор: третий вариант прошлого хода
	// и третий этого — разные ходы, и оставленная подсветка исполнила бы не то,
	// что игрок выбирал.
	m.selected = noSelection
}
