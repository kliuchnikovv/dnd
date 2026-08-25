package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// applyUnlocksFor открывает то, что было заперто выученным фактом. Это и
// превращает набор проверок в расследование: узнал одно — появилось, о чём
// спрашивать и куда идти.
func (g *Game) applyUnlocksFor(f store.FactID) {
	if g.unlocked == nil {
		g.unlocked = map[string]bool{}
	}
	for _, u := range g.DB.Unlocks[f] {
		if u.UnlocksKind == "node" {
			// Новый смысл: факт не «разрешает войти», а рассказывает о месте.
			g.knowPlace(store.NodeID(u.UnlocksID))
			continue
		}
		g.unlocked[u.UnlocksKind+":"+u.UnlocksID] = true
	}
}

func (g *Game) Unlocked(kind, id string) bool { return g.unlocked[kind+":"+id] }

// ReachableNodes — смежные узлы, открытые к посещению. Узел, не упомянутый ни
// в одном fact_unlocks, считается открытым изначально.
func (g *Game) ReachableNodes() []store.NodeID {
	var out []store.NodeID
	for _, n := range g.DB.Locations[g.Node].Adjacent {
		if g.nodeLocked(n) && !g.Unlocked("node", string(n)) {
			continue
		}
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (g *Game) nodeLocked(n store.NodeID) bool {
	for _, us := range g.DB.Unlocks {
		for _, u := range us {
			if u.UnlocksKind == "node" && u.UnlocksID == string(n) {
				return true
			}
		}
	}
	return false
}
