package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// Clocks — часы давления. Заполнение не проигрыш: оно меняет мир через
// Consequence, а mandatory-путь к каждому факту сохраняется по построению дела.
type Clocks struct {
	db    *store.DB
	fired map[store.ClockID]bool
}

func NewClocks(db *store.DB) *Clocks {
	return &Clocks{db: db, fired: map[store.ClockID]bool{}}
}

// Tick продвигает часы на n сегментов и возвращает последствия, сработавшие
// именно сейчас. Заполненные часы больше не срабатывают.
func (c *Clocks) Tick(id store.ClockID, n int) []store.Consequence {
	cl, ok := c.db.Clocks[id]
	if !ok {
		return nil
	}
	if cl.Filled >= cl.Segments {
		return nil
	}
	cl.Filled += n
	if cl.Filled > cl.Segments {
		cl.Filled = cl.Segments
	}
	if cl.Filled < cl.Segments || c.fired[id] {
		return nil
	}
	c.fired[id] = true
	return []store.Consequence{cl.OnFill}
}

// TickAll продвигает все часы с политикой on_cost — цена провала бьёт по
// каждому из них, если дело не указало иного.
func (c *Clocks) TickAll(n int) []store.Consequence {
	var out []store.Consequence
	for _, cl := range c.Snapshot() {
		if cl.TickPolicy == "on_cost" {
			out = append(out, c.Tick(cl.ID, n)...)
		}
	}
	return out
}

func (c *Clocks) Filled(id store.ClockID) bool {
	cl, ok := c.db.Clocks[id]
	return ok && cl.Filled >= cl.Segments
}

// Snapshot возвращает часы в стабильном порядке по ID — для вывода и для
// детерминированного обхода.
func (c *Clocks) Snapshot() []store.Clock {
	out := make([]store.Clock, 0, len(c.db.Clocks))
	for _, cl := range c.db.Clocks {
		out = append(out, *cl)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Stalled — часы давления вышли, а верного обвинения нет. Это висяк: прогон
// заканчивается, но текстом, а не молчанием. Раскрытое дело висяком не
// становится, сколько бы ни натикало после.
func (g *Game) Stalled() bool {
	if g.solved {
		return false
	}
	any := false
	for _, c := range g.C.Snapshot() {
		if c.TickPolicy != "on_cost" {
			continue
		}
		any = true
		if c.Filled < c.Segments {
			return false
		}
	}
	return any
}
