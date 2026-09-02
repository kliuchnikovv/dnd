// Package deduction — архетип детектива за швом Scenario. Внутри — граф
// фактов не живёт (он в core как утилита знания), а живёт правильный ответ
// обвинения (core/scenarios/deduction/accusation) и условия победы/поражения.
//
// Accuse, AccusationResult, Hint/noteTurn (счётчик холостых ходов) остались
// методами core.Game: Go не позволяет объявить метод на чужом типе, а сами
// эти механики читают только приватное состояние Game (g.truth, g.dry,
// g.hinted, g.visited) и уже сегодня используются вне зависимости от
// архетипа (core/turn.go зовёт noteTurn на каждом жёстком глаголе). Здесь —
// только тонкая обвязка над уже существующим публичным API Game.
package deduction

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
)

func init() {
	core.RegisterScenario(core.ScenarioDeduction, func() core.Scenario { return &scenario{} })
}

type scenario struct{}

func (s *scenario) Kind() core.ScenarioKind { return core.ScenarioDeduction }

// ExtraVerbs — глаголов, специфичных именно для детектива и не входящих в
// core.Verbs, сегодня нет: accuse — команда CLI-уровня (cli/accuse.go), а не
// запись в реестре глаголов ядра.
func (s *scenario) ExtraVerbs() []core.VerbDef { return nil }

// Victory — выиграно, если дело закрыто верным обвинением. Проверка и текст
// развязки уже живут в core.Game (Solved/Aftermath); здесь — только
// переиспользование.
func (s *scenario) Victory(g *core.Game) (bool, string) {
	return g.Solved(), g.Aftermath()
}

// Defeat — проиграно, если часы давления вышли, а верного обвинения нет.
// core.Game.Stalled уже реализует ровно эту проверку.
func (s *scenario) Defeat(g *core.Game) (bool, string) {
	return g.Stalled(), g.ColdCase()
}

// Panel — казбук с секциями who/how/when/why. Наполним в Task 20 (панель
// пока — заглушка одной секции с числом фактов, чтобы Kind() был корректен).
func (s *scenario) Panel(g *core.Game) core.Panel {
	return core.Panel{Sections: []core.PanelSection{{Kind: "casebook"}}}
}

// NPCTurn — у детектива NPC ходов не бывает. Всегда false.
func (s *scenario) NPCTurn(*core.Game, store.EntityID) (core.Intent, bool) {
	return core.Intent{}, false
}
