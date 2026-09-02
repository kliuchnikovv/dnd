package dnd5e

import (
	"sort"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// Participant — участник ини-проверки.
type Participant struct {
	ID     store.EntityID
	DexMod int
}

// Initiative — порядок ходов. d20 + DexMod, сортировка по убыванию;
// ties разрешаются последующим d20 (не сохраняются).
func Initiative(ps []Participant, d core.Dice) []store.EntityID {
	scored := make([]struct{ id store.EntityID; total int }, len(ps))
	for i, p := range ps {
		scored[i] = struct{ id store.EntityID; total int }{p.ID, d.Roll(1, 20) + p.DexMod}
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].total == scored[j].total {
			return d.Roll(1, 20) > d.Roll(1, 20)
		}
		return scored[i].total > scored[j].total
	})
	out := make([]store.EntityID, len(scored))
	for i, s := range scored {
		out[i] = s.id
	}
	return out
}
