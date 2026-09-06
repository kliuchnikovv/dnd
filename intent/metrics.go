package intent

import (
	"sort"
	"sync"

	"github.com/kliuchnikovv/dnd/core"
)

// Observed — чем кончился разбор с точки зрения метрики. Три значения, а не
// два: с тех пор как приземляется всё (ADR-0003, T1), «не принято» перестало
// означать «не понято». Проба — понятый ввод, которому словарь не нашёл
// глагола; считать её отказом значит загнать долю отказов в единицу и потерять
// единственный сигнал о том, где словарь узок.
type Observed int

const (
	ObservedAccepted Observed = iota
	ObservedRejected
	ObservedProbe
)

// Metrics — intent rejection rate по классу глагола. Метрика продуктовая:
// высокая доля в одном классе означает, что словарь узок именно там, а не
// вообще. Без разбивки по классу сигнал бесполезен.
type Metrics struct {
	mu       sync.Mutex
	proposed map[core.VerbClass]int
	rejected map[core.VerbClass]int
	// classless — ввод, для которого модель вообще не предложила глагола.
	classlessTotal    int
	classlessRejected int
	// probes — сколько вводов приземлилось свободной пробой. Отдельным
	// счётчиком, а не внутри classless: проба и непонятое — разные вещи, и
	// сложенные вместе они не измеряют ни одну из них.
	probes int
}

func NewMetrics() *Metrics {
	return &Metrics{
		proposed: map[core.VerbClass]int{},
		rejected: map[core.VerbClass]int{},
	}
}

// Observe записывает исход разбора. class пуст, если глагол не предлагался.
//
// Проба в знаменатель не идёт вовсе: доля отказов отвечает на вопрос «как часто
// словарь не справился», а проба — это когда он не справился и это ни для кого
// не стоило хода. Смешав их, метрика перестала бы отличать узкий словарь от
// широкого исследования.
func (m *Metrics) Observe(class core.VerbClass, out Observed) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if out == ObservedProbe {
		m.probes++
		return
	}
	if class == "" {
		m.classlessTotal++
		if out == ObservedRejected {
			m.classlessRejected++
		}
		return
	}
	m.proposed[class]++
	if out == ObservedRejected {
		m.rejected[class]++
	}
}

// Probes — сколько вводов приземлилось свободной пробой. Число само по себе
// продуктовое: высокое означает, что игрок исследует мимо словаря, и это
// заявка на новые глаголы, а не поломка.
func (m *Metrics) Probes() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.probes
}

// Rate — доля отказов в классе. Ноль наблюдений даёт ноль, а не NaN:
// метрику читает дашборд, а не только тест.
func (m *Metrics) Rate(class core.VerbClass) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.proposed[class] == 0 {
		return 0
	}
	return float64(m.rejected[class]) / float64(m.proposed[class])
}

// Overall — доля вводов, не ставших действием, включая те, где глагол не
// предлагался вовсе: игроку всё равно, почему его не поняли.
//
// Пробы в знаменатель не идут: проба — это понятый ввод, которому словарь не
// нашёл глагола, и она ни для кого не стоила хода. Их число читается отдельно,
// через Probes.
func (m *Metrics) Overall() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	total, rejected := m.classlessTotal, m.classlessRejected
	for c, n := range m.proposed {
		total += n
		rejected += m.rejected[c]
	}
	if total == 0 {
		return 0
	}
	return float64(rejected) / float64(total)
}

// Observations — сколько вводов наблюдалось. Метрика без числа наблюдений
// вводит в заблуждение: «0% непонятого» на пустой выборке читается как успех.
//
// Пробы здесь СЧИТАЮТСЯ, хотя в знаменатель Overall и не идут. Это разные
// вопросы: Overall спрашивает «как часто словарь не справился», Observations —
// «разбирался ли свободный текст вообще». Прогон из одних проб — это разбор
// каждый ход, и отчёт, объявивший его тишиной, врал бы о самом частом исходе.
func (m *Metrics) Observations() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.classlessTotal + m.probes
	for _, n := range m.proposed {
		total += n
	}
	return total
}

// Breaches возвращает классы, где доля отказов выше порога. Цель дизайна —
// меньше 15% в целом и меньше 25% в любом классе.
func (m *Metrics) Breaches(perClass float64) []core.VerbClass {
	m.mu.Lock()
	classes := make([]core.VerbClass, 0, len(m.proposed))
	for c := range m.proposed {
		classes = append(classes, c)
	}
	m.mu.Unlock()

	var out []core.VerbClass
	for _, c := range classes {
		if m.Rate(c) > perClass {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
