package e2e

import (
	"math/rand"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/rules/threshold"
)

// Модификатор, срабатывающий почти всегда, — это неверно назначенный порог:
// он не несёт информации, и место такой константы в пороге, а не в таблице.
// Тест ловит весь класс автоматически, до того как таблица ожидаемого
// распределения разойдётся с реальностью.
//
// Нижняя граница проверяется только для факторов, которые обязаны работать в
// расследовании. Численное превосходство, разница Tier и укрытие боевые: в
// деле без боя они не срабатывают ни разу, и это правильно, а не дефект
// спецификации. Их частота печатается, но не судится.
const (
	factorTooOften = 0.60
	factorTooRare  = 0.05
)

// Нижняя граница судит только «adverse»: среда есть в каждом деле, и если она
// не срабатывает — сломан либо разбор тегов, либо разметка узлов. «Не
// обнаружен» после починки стал условным достижением, а не базовой линией, и в
// деле без враждебных срабатывает редко по построению.
var detectiveFactors = map[string]bool{"adverse": true}

func TestNoSituationalFactorFiresAlmostAlways(t *testing.T) {
	fired := map[string]int{}
	rolls := 0

	for seed := int64(0); seed < 200; seed++ {
		g := newGame(t, seed)
		r := rand.New(rand.NewSource(seed))
		for i := 0; i < 40; i++ {
			in, ok := randomProbe(g, r)
			if !ok {
				moveRandomly(g, r)
				continue
			}
			for name, value := range threshold.SituationalFactors(in, g.SceneView(in)) {
				if value != 0 {
					fired[name]++
				}
			}
			rolls++
			g.Apply(in)
			// Ходить по узлам обязательно: выборка из одного стартового узла
			// измеряет не игру, а его теги.
			if r.Intn(3) == 0 {
				moveRandomly(g, r)
			}
		}
	}

	for _, name := range threshold.FactorNames() {
		rate := float64(fired[name]) / float64(rolls)
		t.Logf("%-12s срабатывает в %.1f%% бросков", name, rate*100)
		if rate > factorTooOften {
			t.Errorf("%s срабатывает в %.1f%% бросков — это не модификатор, а константа",
				name, rate*100)
		}
		if detectiveFactors[name] && rate < factorTooRare {
			t.Errorf("%s срабатывает в %.1f%% бросков — фактор мёртв", name, rate*100)
		}
	}
}

// randomProbe собирает осмысленную жёсткую пробу: присутствующая цель, тема из
// банка знаний парти.
func randomProbe(g *core.Game, r *rand.Rand) (core.Intent, bool) {
	here := g.DB.EntitiesAt(g.Node)
	bank := g.K.TopicBank()
	if len(here) == 0 || len(bank) == 0 {
		return core.Intent{}, false
	}
	verbs := []core.Verb{"examine", "question", "search", "stake_out", "tail", "sneak"}
	return core.Intent{
		Verb:  verbs[r.Intn(len(verbs))],
		Actor: g.Actor,
		Args: core.Args{
			Target: here[r.Intn(len(here))].ID,
			Topic:  bank[r.Intn(len(bank))],
		},
	}, true
}
