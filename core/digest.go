package core

import (
	"fmt"
	"sort"
	"strings"
)

// Дайджест состояния — доверенная сводка того, что consistency-guard обязан
// защитить от вранья модели: чем владеет парти, где она, чем ранена, что знает,
// как заполнены часы и чем кончился ход. Второй вектор газлайтинга — не «выдумай
// дело» (его ловит утечка), а «соври про состояние»: «у тебя есть ключ», «ты уже
// в кузнице», «получилось», «часы полны» — вопреки стору.
//
// Асимметрия здесь другая, чем у утечки. Бытовую деталь в сомнении гвард
// пропускает, но состояние — это ДАННЫЕ, проверяемые точно: противоречие
// дайджесту ловится всегда, без «в сомнении пропусти».
//
// Дайджест — чистая функция от (состояние ядра, исход хода). Правды дела
// (accusation.Truth, неизвестные парти факты, держатели) он не читает и читать
// не вправе: в сводке только то, что парти уже знает и может проверить сама.

// DigestClock — часы давления в дайджесте: сколько сегментов из скольких.
type DigestClock struct {
	Name     string
	Filled   int
	Segments int
}

// DigestOutcome — исход только что сделанного хода. Отказ и провал различены:
// «мир не принял» — не то же, что «попробовал и не вышло».
type DigestOutcome struct {
	// Refused — мир не принял действие: ход не потрачен, состояние не менялось.
	Refused bool
	// Rolled — ход бросал кость. Без броска класса исхода нет (социальный ход,
	// ветка без броска), и утверждать «получилось/провал» не о чем.
	Rolled bool
	Class  Outcome
	// Learned — ключи фактов, открытых ИМЕННО этим ходом. «Что открылось» из
	// дизайна: об этом персонаж вправе говорить, о необнаруженном — нет.
	Learned []string
}

// Digest — детерминированная сводка проверяемого состояния. Поля, а не готовый
// текст: так «меняется вслед за состоянием» и «не течёт truth» проверяются по
// значениям, а формат для промпта остаётся тонким слоем (Lines).
type Digest struct {
	Carried []string // имена предметов в инвентаре парти, в стабильном порядке
	Harm    int
	Grit    int
	Node    string   // название текущего узла
	Known   []string // ключи известных парти фактов, отсортированы
	Clocks  []DigestClock
	Outcome DigestOutcome
}

// StateDigest собирает сводку состояния. Чистая функция: g даёт устойчивое
// состояние (инвентарь, раны, узел, известное, часы), res — исход хода, который
// в g не хранится. Оба детерминированы, и порядок известного и часов стабилен.
func StateDigest(g *Game, res TurnResult) Digest {
	d := Digest{
		Node: g.DB.Locations[g.Node].Name,
	}
	for _, it := range g.Carried() {
		d.Carried = append(d.Carried, it.Name)
	}
	if ch := g.DB.Characters[g.Actor]; ch != nil {
		d.Harm, d.Grit = ch.Harm, ch.Grit
	}
	// Известное — только ключи фактов, которые парти УЖЕ знает (party_knowledge).
	// Неизвестного здесь нет по построению: банк тем строится из знания, а не из
	// всех фактов дела.
	for _, f := range g.K.TopicBank() {
		if key := g.DB.Facts[f].Key; key != "" {
			d.Known = append(d.Known, key)
		}
	}
	sort.Strings(d.Known)
	// Snapshot уже отсортирован по ID: порядок часов стабилен.
	for _, cl := range g.C.Snapshot() {
		d.Clocks = append(d.Clocks, DigestClock{
			Name: cl.Name, Filled: cl.Filled, Segments: cl.Segments,
		})
	}
	d.Outcome = DigestOutcome{Refused: res.Refused}
	if res.Res != nil {
		d.Outcome.Rolled = true
		d.Outcome.Class = res.Res.Class
	}
	for _, l := range res.Learned {
		if key := g.DB.Facts[l.Fact].Key; key != "" {
			d.Outcome.Learned = append(d.Outcome.Learned, key)
		}
	}
	return d
}

// Lines разворачивает дайджест в строки для материала guard. Формат ровный и
// служебный: гвард сверяет реплику с этими утверждениями, а не читает прозу.
func (d Digest) Lines() []string {
	out := make([]string, 0, 8)
	if len(d.Carried) > 0 {
		out = append(out, "Несёт при себе: "+strings.Join(d.Carried, ", "))
	} else {
		out = append(out, "При себе ничего нет")
	}
	out = append(out, fmt.Sprintf("Состояние: раны %d, стойкость %d", d.Harm, d.Grit))
	if d.Node != "" {
		out = append(out, "Сейчас находится: "+d.Node)
	}
	if len(d.Known) > 0 {
		out = append(out, "Знает по делу: "+strings.Join(d.Known, "; "))
	} else {
		out = append(out, "По делу пока не знает ничего")
	}
	for _, c := range d.Clocks {
		out = append(out, fmt.Sprintf("Часы «%s»: %d из %d", c.Name, c.Filled, c.Segments))
	}
	out = append(out, d.Outcome.line())
	return out
}

// line описывает исход хода одной строкой.
func (o DigestOutcome) line() string {
	switch {
	case o.Refused:
		return "Итог хода: мир не принял действие (отказ)"
	case o.Rolled:
		s := "Итог хода: " + o.Class.String()
		if len(o.Learned) > 0 {
			s += "; открылось: " + strings.Join(o.Learned, "; ")
		}
		return s
	case len(o.Learned) > 0:
		return "Итог хода: открылось: " + strings.Join(o.Learned, "; ")
	default:
		return "Итог хода: без изменений в состоянии"
	}
}
