package adventure

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

// npcTurn — ход монстра. В MVP цель одна — игрок: враг в том же узле
// атакуется, иначе монстр делает шаг в его сторону по смежности узлов.
func npcTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
	npc, ok := g.DB.Entities[id]
	if !ok {
		return core.Intent{}, false
	}
	target := store.EntityID(g.Actor)
	player, ok := g.DB.Entities[target]
	if !ok {
		return core.Intent{}, false
	}
	if npc.Node == player.Node {
		return core.Intent{Verb: "attack", Actor: store.CharacterID(id),
			Args: core.Args{Target: target}}, true
	}
	next, ok := stepToward(g, npc.Node, player.Node)
	if !ok {
		return core.Intent{}, false
	}
	return core.Intent{Verb: "move_zone", Actor: store.CharacterID(id),
		Args: core.Args{Node: next}}, true
}

// stepToward — первый шаг кратчайшего пути от from к to по Location.Adjacent
// (простой BFS). Второй результат ложен, если пути нет.
func stepToward(g *core.Game, from, to store.NodeID) (store.NodeID, bool) {
	if from == to {
		return "", false
	}

	// firstStep — через какой из соседей from мы попали в узел ключа.
	// Запоминается на входе в очередь, чтобы в конце BFS вернуть не сам
	// целевой узел, а первый шаг к нему.
	firstStep := map[store.NodeID]store.NodeID{}
	visited := map[store.NodeID]bool{from: true}
	queue := []store.NodeID{from}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		loc, ok := g.DB.Locations[cur]
		if !ok {
			continue
		}
		for _, next := range loc.Adjacent {
			if visited[next] {
				continue
			}
			visited[next] = true
			if cur == from {
				firstStep[next] = next
			} else {
				firstStep[next] = firstStep[cur]
			}
			if next == to {
				return firstStep[next], true
			}
			queue = append(queue, next)
		}
	}
	return "", false
}
