// Package view5e — презентационная ось правила dnd5e (спека 2/3, ADR-0010).
//
// Отдельный пакет, а не файл в rules/dnd5e: rules/dnd5e тянет ядро без
// импорта view (это позволяет cases.Load через core/scenarios/adventure
// цеплять dnd5e-механику из тестов пакета view без цикла). Импорт связки
// «механика + меры» делает вход в приложение: cmd/server и cmd/dnd
// blank-импортят и rules/dnd5e (init регистрирует core.RuleSystem), и этот
// пакет (init регистрирует view.Ruleset).
package view5e

import (
	"sort"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/store"
	"github.com/kliuchnikovv/dnd/view"
)

func init() {
	view.RegisterRuleset(core.RulesetDND5e, func() view.Ruleset { return Ruleset{} })
}

// Ruleset — ось view-правила dnd5e. Меры собираются из data-модели d20-
// боёвки: HP/MaxHP актёра как сущности (единственный источник правды
// здоровья в D&D-механике — та же таблица, что читает adventure.Defeat) и
// боевые состояния из store.DB.Conditions. Полный лист, карта, инвентарь и
// инициатива — objective-панель приключения (core.Scenario.Panel →
// view.panelFromCore), не меры: меры — то, что должно быть «на виду».
type Ruleset struct{}

// Сигнатура view.Ruleset обязана держаться на этом типе: разойдись —
// сборка вида перестанет компилироваться здесь, а не молча у вызова.
var _ view.Ruleset = Ruleset{}

// Meters — меры dnd5e: HP и активные состояния. HP всплывает при любом
// ранении (HP < MaxHP): полное здоровье не занимает экран, минимализм —
// задача правила, а не клиента. Состояния всплывают всегда (условие тикает
// раундами и по определению актуально сейчас); значение меры — оставшиеся
// раунды.
//
// Актёр в HP-модели адресуется как сущность (store.EntityID(g.Actor)) — та
// же адресация, что у ядерной мутации MutHPDelta и у adventure.Defeat:
// другой таблицы для HP в d20-механике нет.
func (Ruleset) Meters(g *core.Game) []view.Meter {
	actor := store.EntityID(g.Actor)
	ent := g.DB.Entities[actor]

	var out []view.Meter
	if ent.MaxHP > 0 {
		max := ent.MaxHP
		out = append(out, view.Meter{
			Label:   "HP",
			Kind:    "hp",
			Value:   ent.HP,
			Max:     &max,
			Surface: ent.HP < ent.MaxHP,
		})
	}

	conds := g.DB.Conditions[actor]
	if len(conds) == 0 {
		return out
	}
	keys := make([]string, 0, len(conds))
	for k := range conds {
		keys = append(keys, k)
	}
	// Порядок стабильный: без сортировки клиент видел бы меры разного
	// порядка от кадра к кадру, и diff turn-view становился бы шумным.
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, view.Meter{
			Label:   k,
			Kind:    "condition",
			Value:   conds[k],
			Surface: true,
		})
	}
	return out
}
