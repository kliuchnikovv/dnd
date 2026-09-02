package adventure_test

// Гейт фазы 4: мини-приключение играется целиком, от старта до Victory=true,
// через настоящий core.Game.Apply — не юнит-тест одного узла, а доказательство,
// что фазы 1-4 (ядро, глаголы D&D, сценарий adventure, загрузчик кейсов)
// работают вместе на одном прогоне.

import (
	"encoding/json"
	"testing"

	"github.com/kliuchnikovv/dnd/cases"
	"github.com/kliuchnikovv/dnd/core"
	_ "github.com/kliuchnikovv/dnd/core/scenarios/adventure"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/dnd5e"
	"github.com/kliuchnikovv/dnd/store"
)

// gameFor собирает Game из мини-кейса. cases.Parse не умеет разбирать поле
// "rules" (в схеме его вовсе нет) — единственная система правил, которую
// понимает загрузчик, назначается тем же путём, что и в e2e/forte_merlo_test.go:
// вызывающий сам подставляет cfg.Rules и cfg.Dice после Load.
func gameFor(t *testing.T, seed int64) *core.Game {
	t.Helper()
	cfg, err := cases.Load("../../../cases/testdata/mini_adventure.json")
	if err != nil {
		t.Fatalf("загрузка мини-кейса: %v", err)
	}
	cfg.Rules = dnd5e.New()
	cfg.Dice = dice.NewSource(seed).Stream("resolve")
	// Character в case.json больше не живёт: лист героя кладёт вызывающий —
	// тот же лист, что раньше нёс сам мини-кейс.
	cfg.DB.SaveCharacter(&store.Character{
		ID:      cfg.Actor,
		Ruleset: "dnd5e",
		Sheet: json.RawMessage(`{
			"str": 14, "dex": 14, "prof": 2,
			"skills": [], "saves": [],
			"max_hp": 10, "ac": 10, "speed": 30,
			"weapons": [
				{"name": "sword", "reach": "melee", "attack": "str", "damage": "1d6+str"}
			]
		}`),
	})
	return core.NewGame(*cfg)
}

// moveAndArrive — move_zone через Apply плюс перестановка позиции. Apply сам
// узел не переставляет: это ответственность слоя над ядром (см.
// cli/repl.go:afterAction), и тест, вызывающий Apply напрямую, обязан
// исполнить тот же контракт, что и cli.Session.
func moveAndArrive(t *testing.T, g *core.Game, node store.NodeID) {
	t.Helper()
	res := g.Apply(core.Intent{Verb: "move_zone", Actor: g.Actor, Args: core.Args{Node: node}})
	if res.Refused {
		t.Fatalf("move_zone в %s отказан: %s", node, res.Refusal)
	}
	if res.Res == nil || res.Res.Class < core.OutcomePartial {
		t.Fatalf("move_zone в %s не удался: %+v", node, res.Res)
	}
	g.MoveTo(node)
}

// TestMiniAdventureReachesVictory — план: атаковать орка до смерти, забрать
// самоцвет с его тела, вернуться в лагерь. Единственная проверка гейта —
// Victory=true в конце; seed фиксирован, прогон детерминирован.
func TestMiniAdventureReachesVictory(t *testing.T) {
	g := gameFor(t, 7)

	if g.Node != "n_start" {
		t.Fatalf("старт не n_start: %q", g.Node)
	}

	// Идём в логово орка.
	moveAndArrive(t, g, "n_end")

	// Бой начинается авторски-вручную (вариант A из брифа задачи): узел
	// боевой машинерии, которая сама решала бы, когда начинать encounter по
	// сцене автора, — задача будущих кейсов. Здесь порядок ходов и вход в бой
	// заданы прямо полем Game.Encounter, потому что фикстура не содержит
	// авторского триггера (MutEncounterStart через данные кейса).
	g.Encounter = &core.Encounter{Order: []store.EntityID{
		store.EntityID(g.Actor), "e_orc",
	}}

	// Бьём орка, пока не падёт. HP=4, AC=10 у обоих — пары ударов хватает.
	// Границу в 15 попыток кладём с большим запасом: ниже неё детерминированный
	// seed=7 укладывается за пару ходов, а граница нужна только на случай,
	// если кто-то поменяет seed и забудет проверить прогон.
	const maxRounds = 15
	orcDown := false
	for i := 0; i < maxRounds; i++ {
		res := g.Apply(core.Intent{Verb: "attack", Actor: g.Actor, Args: core.Args{Target: "e_orc"}})
		if res.Refused {
			t.Fatalf("attack на орка отказан на попытке %d: %s", i, res.Refusal)
		}
		orc := g.DB.Entities["e_orc"]
		if orc.HP <= 0 {
			orcDown = true
			break
		}
	}
	if !orcDown {
		t.Fatalf("орк не пал за %d попыток атаки — seed требует пересмотра", maxRounds)
	}

	// Орк мёртв — бой закончен. Реальный кейс закрыл бы его MutEncounterEnd
	// из данных сценария; здесь тот же исход выставляется тем же полем, каким
	// бой был открыт — Game.Encounter экспортирован намеренно для таких
	// сценарных развязок.
	g.Encounter = nil

	// Осматриваем тело — держатель факта f_gem обязателен (mandatory),
	// поэтому выдача идёт без броска: самоцвет достаётся детерминированно.
	res := g.Apply(core.Intent{Verb: "examine", Actor: g.Actor, Args: core.Args{Target: "e_gem_body"}})
	if res.Refused {
		t.Fatalf("examine тела орка отказан: %s", res.Refusal)
	}
	if !g.Carries("i_gem") {
		t.Fatalf("после examine самоцвет не в инвентаре: learned=%v", res.Learned)
	}

	// Возвращаемся в лагерь с добычей.
	moveAndArrive(t, g, "n_start")

	won, why := g.Scenario.Victory(g)
	if !won {
		t.Fatalf("Victory не наступила: %q", why)
	}
}
