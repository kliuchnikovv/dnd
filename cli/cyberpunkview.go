package cli

import (
	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/rules/cyberpunk"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

// Реф-конфигурация вида для правила «Cyberpunk RED».
//
// Здесь и только здесь живёт словарь правила cyberpunk на экране: меры
// Humanity, HP, Seriously Wounded, Heat. Пакет view их не знает (агностичная
// ось), а сам rules/cyberpunk знать про view не должен — иначе шов зашумит.
// Реф-конфигурация — тонкий адаптер: читает лист персонажа, зовёт формулы
// правила (MaxHP, Humanity), возвращает готовые меры.

var _ view.Ruleset = CyberpunkRuleset{}

func init() {
	view.RegisterRuleset(core.RulesetCyberpunk, func() view.Ruleset { return CyberpunkRuleset{} })
}

// CyberpunkRuleset — ось ПРАВИЛА RED. Называет меры и решает по состоянию,
// какую поднять. Surface — от порога: Humanity видна всегда (сигнатурная
// мера); HP — всегда (боёвка — центральный трек); Seriously Wounded —
// когда HP ≤ ½ (штраф уже действует); Heat — когда ≥ 1 (розыск начался).
type CyberpunkRuleset struct{}

// Meters — четыре RED-меры, поднятые из состояния актёра. Heat — house-rule
// (наше дополнение под MMO-связь мира, а не RED). Именно Humanity/Heat,
// которых нет у threshold и dnd5e, доказывают, что view-ось расширилась
// (см. §3.5 хендоффа cyberpunk).
func (CyberpunkRuleset) Meters(g *core.Game) []view.Meter {
	sheet, _ := cyberpunk.ParseSheet(activeSheet(g))
	maxHP := sheet.MaxHP()
	// В store.Character.Harm — «заполненные ячейки урона»; для cyberpunk это
	// количество HP, вычтенных из максимума. hp = max − harm.
	ch := g.DB.Characters[g.Actor]
	harm := 0
	if ch != nil {
		harm = ch.Harm
	}
	hp := maxHP - harm
	if hp < 0 {
		hp = 0
	}

	humanity := sheet.Humanity()
	humanityMax := sheet.EMP * 10

	out := []view.Meter{
		{Label: "Humanity", Kind: "humanity", Value: humanity, Max: &humanityMax, Surface: true},
		{Label: "HP", Kind: "hp", Value: hp, Max: &maxHP, Surface: true},
	}
	if cyberpunk.SeriouslyWounded(hp, maxHP) {
		full := 1
		out = append(out, view.Meter{
			Label: "Seriously Wounded", Kind: "wounded",
			Value: 1, Max: &full, Surface: true,
		})
	}
	// Heat — уровень розыска. Хранения на персонаже в фазе A нет: значение
	// приходит из часов дела с известным ID «heat» (если автор кейса такие
	// завёл). Отсутствие часов → мера не всплывает — экран чист, пока
	// розыск не начали (см. §3.5: наше house-rule, помечено сверху).
	if heat, ok := heatValue(g); ok {
		heatMax := heatMaxValue(g)
		out = append(out, view.Meter{
			Label: "Heat", Kind: "heat", Value: heat, Max: &heatMax, Surface: true,
		})
	}
	return out
}

// activeSheet — сырьё листа актёра из базы. Пусто, если персонаж не найден:
// нулевой лист безопасен для формул RED.
func activeSheet(g *core.Game) []byte {
	ch, ok := g.DB.CharacterByID(g.Actor)
	if !ok || ch == nil {
		return nil
	}
	return ch.Sheet
}

// heatValue — «розыск» из часов дела с ID "heat"; ok=false, если таких
// часов не заведено. Знать про часы правило не считается нарушением
// изоляции: часы — общий примитив ядра, а «heat» как имя — соглашение
// киберпанк-кейсов (house-rule), не выдумка на клиенте.
func heatValue(g *core.Game) (int, bool) {
	for _, c := range g.C.Snapshot() {
		if c.ID == store.ClockID("heat") {
			return c.Filled, true
		}
	}
	return 0, false
}

func heatMaxValue(g *core.Game) int {
	for _, c := range g.C.Snapshot() {
		if c.ID == store.ClockID("heat") {
			return c.Segments
		}
	}
	return 0
}
