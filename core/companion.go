package core

import (
	"sort"

	"github.com/kliuchnikovv/dnd/store"
)

// HintAfter — сколько ходов подряд впустую терпит напарник, прежде чем
// заговорить. Подсказка на каждом ходу — не помощь, а чтение решения вслух;
// подсказка никогда — это игрок, застрявший в первой сессии и закрывший
// консоль.
const HintAfter = 3

// Hint возвращает реплику напарника, если расследование встало. Указывает она
// на ЦЕЛЬ, а не на ответ: строку пишет автор дела, движок только выбирает
// момент и адресата.
//
// Выбирается первый неизвестный факт, к держателю которого можно подойти
// прямо сейчас. Порядок обхода стабилен: одна и та же ситуация даёт одну и ту
// же подсказку, иначе прогон перестаёт быть воспроизводимым.
func (g *Game) Hint() (string, bool) {
	if g.Companion == "" || g.dry < HintAfter {
		return "", false
	}
	ids := make([]store.FactID, 0, len(g.Hints))
	for id := range g.Hints {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		if g.K.Knows(id) || g.hinted[id] {
			continue
		}
		if !g.holderReachable(id) {
			continue
		}
		g.hinted[id] = true
		g.dry = 0
		return g.Hints[id], true
	}
	return "", false
}

// holderReachable — держатель факта стоит в текущем узле или в соседнем,
// открытом. Подсказывать про склад, куда ещё нет повода идти, значит толкать
// игрока в отказ.
func (g *Game) holderReachable(f store.FactID) bool {
	for _, h := range g.DB.HoldersOf(f) {
		e, ok := g.DB.Entities[h.HolderID]
		if !ok {
			continue
		}
		if e.Node == g.Node {
			return true
		}
		for _, n := range g.ReachableNodes() {
			if e.Node == n {
				return true
			}
		}
	}
	return false
}

// noteTurn ведёт счётчик ходов вхолостую. Считаются только жёсткие пробы:
// осмотреться и подумать — не «застрял».
func (g *Game) noteTurn(def VerbDef, learned int) {
	if !def.Hard {
		return
	}
	if learned > 0 {
		g.dry = 0
		return
	}
	g.dry++
}
