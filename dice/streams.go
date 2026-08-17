// Package dice реализует именованные RNG-стримы. Ядро объявляет интерфейс
// Dice, но math/rand живёт здесь: core обязан оставаться свободным от костей.
package dice

import (
	"hash/fnv"
	"math/rand"
)

// Source раздаёт независимые стримы от одного seed. Стрим с именем name
// засеян hash(seed, name), поэтому добавление броска в одной подсистеме не
// сдвигает последовательность в другой.
type Source struct {
	seed    int64
	streams map[string]*Stream
}

func NewSource(seed int64) *Source {
	return &Source{seed: seed, streams: map[string]*Stream{}}
}

// Stream возвращает стрим по имени, создавая его при первом обращении.
// Повторный вызов с тем же именем отдаёт тот же стрим, а не новый.
func (s *Source) Stream(name string) *Stream {
	if st, ok := s.streams[name]; ok {
		return st
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	st := &Stream{r: rand.New(rand.NewSource(s.seed ^ int64(h.Sum64())))}
	s.streams[name] = st
	return st
}

type Stream struct{ r *rand.Rand }

func (s *Stream) D20() int { return s.r.Intn(20) + 1 }

func (s *Stream) Roll(n, sides int) int {
	total := 0
	for i := 0; i < n; i++ {
		total += s.r.Intn(sides) + 1
	}
	return total
}

// FixedDice подставляет заданные значения — для тестов, где бросок должен быть
// известен заранее. Исчерпав список, повторяет последнее значение.
type FixedDice struct {
	values []int
	i      int
}

func Fixed(values ...int) *FixedDice { return &FixedDice{values: values} }

func (f *FixedDice) D20() int {
	if len(f.values) == 0 {
		return 10
	}
	v := f.values[f.i]
	if f.i < len(f.values)-1 {
		f.i++
	}
	return v
}

func (f *FixedDice) Roll(n, sides int) int { return f.D20() }
