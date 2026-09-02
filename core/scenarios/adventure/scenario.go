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

// Panel — рабочая панель клиента: здоровье, карта смежных узлов, инвентарь,
// а в бою — ещё и инициатива. Секция initiative опускается вне боя целиком,
// а не отдаётся пустой: клиенту незачем гадать, значит ли пустой список
// «бой без участников» или «боя нет».
func (s *scenario) Panel(g *core.Game) core.Panel {
	p := core.Panel{Sections: []core.PanelSection{
		s.healthSection(g),
		s.mapSection(g),
		s.inventorySection(g),
	}}
	if g.Encounter != nil {
		p.Sections = append(p.Sections, s.initiativeSection(g))
	}
	return p
}

// healthSection — HP/MaxHP героя. Источник правды — сущность актёра, та же,
// что читает Defeat: другого места, где хранится HP в D&D-механике, нет.
func (s *scenario) healthSection(g *core.Game) core.PanelSection {
	ent := g.DB.Entities[store.EntityID(g.Actor)]
	return core.PanelSection{Kind: "health", Slots: []core.PanelSlot{
		{Key: "hp", Value: fmt.Sprintf("%d/%d", ent.HP, ent.MaxHP)},
	}}
}

// mapSection — текущий узел и узлы, куда можно шагнуть отсюда прямо сейчас.
func (s *scenario) mapSection(g *core.Game) core.PanelSection {
	slots := []core.PanelSlot{{Key: "here", Value: g.DB.Locations[g.Node].Name}}
	for _, n := range g.DB.Locations[g.Node].Adjacent {
		slots = append(slots, core.PanelSlot{Key: string(n), Value: g.DB.Locations[n].Name})
	}
	return core.PanelSection{Kind: "map", Slots: slots}
}

// inventorySection — что парти несёт при себе, по одному слоту на предмет.
func (s *scenario) inventorySection(g *core.Game) core.PanelSection {
	var slots []core.PanelSlot
	for _, it := range g.Carried() {
		slots = append(slots, core.PanelSlot{Key: string(it.ID), Value: it.Name})
	}
	return core.PanelSection{Kind: "inventory", Slots: slots}
}

// initiativeSection — чей сейчас ход, номер раунда и остаток действий у
// текущего держателя хода. Вызывается только когда g.Encounter не nil.
func (s *scenario) initiativeSection(g *core.Game) core.PanelSection {
	enc := g.Encounter
	slots := []core.PanelSlot{
		{Key: "round", Value: fmt.Sprintf("%d", enc.Round)},
	}
	if cur := enc.Current(); cur != "" {
		slots = append(slots, core.PanelSlot{Key: "current", Value: entityName(g, cur)})
	}
	slots = append(slots,
		core.PanelSlot{Key: "action", Value: fmt.Sprintf("%t", !enc.Actions.Action)},
		core.PanelSlot{Key: "bonus", Value: fmt.Sprintf("%t", !enc.Actions.Bonus)},
	)
	return core.PanelSection{Kind: "initiative", Slots: slots}
}

// entityName — имя сущности для панели; для актёра-персонажа сущности может
// не быть в store.Entities под тем же ID отдельно от Character, поэтому
// свой ID выводим отдельным лейблом, а не пытаемся угадать имя.
func entityName(g *core.Game, id store.EntityID) string {
	if ent, ok := g.DB.Entities[id]; ok && ent.Name != "" {
		return ent.Name
	}
	if id == store.EntityID(g.Actor) {
		return "герой"
	}
	return string(id)
}

func (s *scenario) NPCTurn(g *core.Game, id store.EntityID) (core.Intent, bool) {
	return npcTurn(g, id)
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
