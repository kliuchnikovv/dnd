package core

import "github.com/kliuchnikovv/dnd/store"

type RestKind int

const (
	RestShort RestKind = iota
	RestLong
)

// GritMax — потолок push-ресурса. Значение принадлежит системе правил, но
// восстановление отдыхом — структурный переход ядра, поэтому потолок объявлен
// здесь и переопределяется данными дела в M1b.
const GritMax = 3

// RestPreview называет часы, которые продвинет длинный отдых. Цена
// предъявляется до решения: платить вслепую игрок не должен.
func (g *Game) RestPreview(kind RestKind) []store.ClockID {
	if kind != RestLong {
		return nil
	}
	var out []store.ClockID
	for _, c := range g.C.Snapshot() {
		if c.TickPolicy == "on_cost" && c.Filled < c.Segments {
			out = append(out, c.ID)
		}
	}
	return out
}

// Rest — короткий возвращает grit, длинный снимает ячейку ранений ценой тика.
func (g *Game) Rest(kind RestKind) TurnResult {
	ch := g.DB.Characters[g.Actor]
	if ch == nil {
		return refuse("некому отдыхать")
	}
	if kind == RestShort {
		ch.Grit = GritMax
		return TurnResult{FlavourKey: "rest.short"}
	}
	if ch.Harm > 0 {
		ch.Harm--
	}
	out := TurnResult{FlavourKey: "rest.long"}
	out.Fired = g.C.TickAll(1)
	g.applyConsequences(out.Fired)
	return out
}
