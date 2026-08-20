package intent

import (
	"sort"
	"sync"

	"github.com/kliuchnikovv/dnd/core"
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
}

func NewMetrics() *Metrics {
	return &Metrics{
		proposed: map[core.VerbClass]int{},
		rejected: map[core.VerbClass]int{},
	}
}

// Observe записывает исход разбора. class пуст, если глагол не предлагался.
func (m *Metrics) Observe(class core.VerbClass, accepted bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if class == "" {
		m.classlessTotal++
		if !accepted {
			m.classlessRejected++
		}
		return
	}
	m.proposed[class]++
	if !accepted {
		m.rejected[class]++
	}
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
func (m *Metrics) Observations() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	total := m.classlessTotal
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
