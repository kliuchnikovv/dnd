package core

import "github.com/kliuchnikovv/dnd/store"

// Compare сопоставляет два известных факта. Броска здесь нет и не будет:
// противоречие между двумя фактами либо есть, либо нет — это свойство данных.
// Игрок сопоставил — игрок заметил.
func (g *Game) Compare(a, b store.FactID) TurnResult {
	if a == b {
		return refuse("сопоставлять факт с самим собой нечего")
	}
	if !g.K.Knows(a) || !g.K.Knows(b) {
		return refuse("оба факта должны быть известны парти")
	}
	for _, c := range g.DB.Contradictions {
		if (c.A == a && c.B == b) || (c.A == b && c.B == a) {
			out := TurnResult{FlavourKey: c.FlavourKey}
			// Вывод — не свидетельство: источником становится сама пара фактов,
			// поэтому запись идёт от служебной сущности рассуждения.
			if g.learn(c.Reveals, ReasoningSource) {
				out.Learned = append(out.Learned, Learned{c.Reveals, ReasoningSource})
			}
			g.applyUnlocksFor(c.Reveals)
			return out
		}
	}
	return TurnResult{FlavourKey: "compare.nothing"}
}

// ReasoningSource — источник фактов, полученных рассуждением, а не
// свидетельством. Рёбер в relations у него нет, поэтому он не мешает
// корроборации и не считается за независимого свидетеля дважды.
const ReasoningSource store.EntityID = "e_reasoning"
