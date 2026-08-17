package core

import "github.com/kliuchnikovv/dnd/store"

// applyMutations применяет обобщённые мутации. Ядро знает имена только двух
// ресурсов — grit и harm, потому что на них ссылается таксономия цены провала.
// Всё прочее приходит от системы правил и молча игнорируется, если ядру
// незнакомо: это не ошибка, а граница ответственности.
func (g *Game) applyMutations(ms []Mutation) {
	ch := g.DB.Characters[g.Actor]
	for _, m := range ms {
		switch m.Kind {
		case MutResource:
			if m.Target == "grit" && ch != nil {
				ch.Grit += m.Delta
				if ch.Grit < 0 {
					ch.Grit = 0
				}
			}
		case MutHarm:
			if c, ok := g.DB.Characters[store.CharacterID(m.Target)]; ok {
				c.Harm += m.Delta
			}
		case MutClock:
			g.C.Tick(store.ClockID(m.Target), m.Delta)
		case MutDisposition:
			g.Disposition[store.EntityID(m.Target)] += m.Delta
		case MutPosition:
			g.Detected = m.Delta < 0
		}
	}
}

// executeCosts исполняет выбранную правилами цену. Возвращает последствия
// заполнившихся часов.
func (g *Game) executeCosts(cs []CostKind, in Intent) []store.Consequence {
	var fired []store.Consequence
	ch := g.DB.Characters[g.Actor]
	for _, c := range cs {
		switch c {
		case CostTickClock:
			fired = append(fired, g.C.TickAll(1)...)
		case CostDebt:
			g.Debts[in.Args.Target]++
		case CostDispositionDown:
			g.Disposition[in.Args.Target]--
		case CostPositionWorse:
			g.Detected = true
		case CostHarmSelf:
			if ch != nil {
				ch.Harm++
			}
		case CostFalseLead, CostHalfEffect, CostResourceSpent:
			// Меняют не мир, а исход хода: отражаются в TurnResult.
		}
	}
	return fired
}

// applyConsequences исполняет срабатывание часов. Держатели могут исчезнуть,
// сущности — озлобиться, но mandatory-путь к каждому факту дело обязано
// сохранить: это проверяет валидатор загрузки.
func (g *Game) applyConsequences(cs []store.Consequence) {
	for _, c := range cs {
		for _, f := range c.RemoveHolders {
			delete(g.DB.Holders, f)
		}
		for _, e := range c.HostileTo {
			g.Disposition[e] -= 2
		}
	}
}
