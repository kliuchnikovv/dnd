// Package core — Scenario. Второй ортогональный шов рядом с RuleSystem
// (§3 спеки, ADR-0006). Ядро видит только интерфейс: конкретный архетип
// (детектив, приключение) живёт за швом в core/scenarios/*.
package core

import "github.com/kliuchnikovv/dnd/store"

type ScenarioKind string

const (
	ScenarioDeduction ScenarioKind = "deduction"
	ScenarioAdventure ScenarioKind = "adventure"
)

// Scenario — что архетип поставляет ядру. Панель и NPCTurn — крючки в такт
// презентации и хода; Victory/Defeat — единственная точка правды об исходе
// прогона.
type Scenario interface {
	Kind() ScenarioKind
	ExtraVerbs() []VerbDef
	Victory(g *Game) (won bool, why string)
	Defeat(g *Game) (lost bool, why string)
	Panel(g *Game) Panel
	NPCTurn(g *Game, id store.EntityID) (Intent, bool)
}

// Panel — дескриптор рабочей панели для клиента. Форма общая, содержимое
// определяет сценарий; клиент рисует по дескриптору, не зная конкретики.
// Наполнение секций/слотов — в Task 20.
type Panel struct {
	Sections []PanelSection
}

type PanelSection struct {
	Kind  string
	Slots []PanelSlot
}

type PanelSlot struct {
	Key   string
	Value string
}

var scenarioRegistry = map[ScenarioKind]func() Scenario{}

// RegisterScenario — зарегистрировать фабрику архетипа. Регистрация одна
// на процесс: init-функция пакета сценария зовёт её при импорте.
func RegisterScenario(k ScenarioKind, factory func() Scenario) {
	scenarioRegistry[k] = factory
}

// LookupScenario — получить экземпляр архетипа по имени. Второй результат
// ложен, если сценарий не зарегистрирован (например, пакет не импортирован).
func LookupScenario(k ScenarioKind) (Scenario, bool) {
	f, ok := scenarioRegistry[k]
	if !ok {
		return nil, false
	}
	return f(), true
}
