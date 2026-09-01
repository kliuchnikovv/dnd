// Package adventure — solo D&D-приключение за швом Scenario. Победа —
// «вернись сюда с этим», поражение — HP≤0 без hit dice. AI монстров живёт
// в ai.go, victory-спецификация — в victory.go, наполнение Panel — здесь.
package adventure

import (
	"fmt"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/rules/dnd5e"
	"github.com/kliuchnikovv/dnd/store"
)

func init() {
	core.RegisterScenario(core.ScenarioAdventure, func() core.Scenario { return &scenario{} })
}

// scenario — архетип приключения. victory настраивается после LookupScenario
// через SetVictoryOn: у самой фабрики регистрации нет доступа к case.json,
// а тип нарочно неэкспортируемый, чтобы cases не знал конкретики архетипа.
type scenario struct {
	victory VictorySpec
}

func (s *scenario) Kind() core.ScenarioKind { return core.ScenarioAdventure }

// ExtraVerbs — боевые и приключенческие глаголы D&D 5e: attack, hide,
// disarm_trap, detect_trap и другие из rules/dnd5e.
func (s *scenario) ExtraVerbs() []core.VerbDef { return dnd5e.Verbs() }

// Victory — определяется case.json → victory: {type,item,node}. Спецификация
// без типа (кейс без поля victory) победу не объявляет никогда.
func (s *scenario) Victory(g *core.Game) (bool, string) {
	return s.victory.check(g)
}

// Defeat — HP игрока ≤ 0. Hit dice в MVP не моделируем: их отсутствие и есть
// условие поражения, а не отдельная проверка сверху.
func (s *scenario) Defeat(g *core.Game) (bool, string) {
	hp := g.DB.Entities[store.EntityID(g.Actor)].HP
	if hp <= 0 {
		return true, "герой пал"
	}
	return false, ""
}

// Panel — заглушка одной секции; наполнение реального содержимого — Task 20.
func (s *scenario) Panel(g *core.Game) core.Panel {
	return core.Panel{Sections: []core.PanelSection{{Kind: "adventure_stub"}}}
}

func (s *scenario) NPCTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
	return npcTurn(g, id)
}

// npcTurn — заглушка хода NPC. Реальная реализация (attack/move_zone по BFS)
// появляется в ai.go (Task 18).
func npcTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
	_, _ = g, id
	return core.Intent{}, false
}

// SetVictoryOn настраивает victory-спецификацию на уже созданном экземпляре
// сценария adventure. Нужен пакету cases: LookupScenario отдаёт core.Scenario
// без доступа к внутренностям, а раскрывать конкретный тип scenario наружу
// значило бы протащить архитектуру архетипа туда, где её знать не должны.
func SetVictoryOn(sc core.Scenario, spec VictorySpec) error {
	a, ok := sc.(*scenario)
	if !ok {
		return fmt.Errorf("adventure: SetVictoryOn вызван не на adventure-сценарии (%T)", sc)
	}
	a.victory = spec
	return nil
}
