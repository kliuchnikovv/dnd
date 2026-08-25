package core

import (
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

// ReachableNodes — места, куда можно пойти: известные парти, кроме того, где
// игрок стоит. Смежность здесь не при чём — гейт стоит на знании (core/places.go).
func (g *Game) ReachableNodes() []store.NodeID {
	var out []store.NodeID
	for _, n := range g.KnownPlaces() {
		if n != g.Node {
			out = append(out, n)
		}
	}
	return out
}
