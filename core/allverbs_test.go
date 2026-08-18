package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/store"
)

// TestEveryVerbResolvesWithoutPanic прогоняет все 30 глаголов через Apply на
// осмысленных аргументах. Проверяется не исход, а отсутствие дыр: паники,
// нулевого результата, необработанной ветки.
func TestEveryVerbResolvesWithoutPanic(t *testing.T) {
	for _, def := range AllVerbs() {
		t.Run(string(def.Verb), func(t *testing.T) {
			g := turnGame(OutcomeSuccess)
			g.K.Learn("f_open", "e_bern")
			in := Intent{Verb: def.Verb, Actor: g.Actor, Args: Args{
				Target: "e_toke", Topic: "f_open", Node: "n_forge",
				Item: "crowbar", Ability: "read_room", Text: "гипотеза",
			}}
			got := g.Apply(in)
			if got.Refused && got.Refusal == "" {
				t.Error("отказ без причины — необработанная ветка")
			}
			if !got.Refused && got.FlavourKey == "" {
				t.Error("действие прошло, но не дало ключа флейвора")
			}
		})
	}
}

func TestTheorizeIsRecordedAndCostsNothing(t *testing.T) {
	g := turnGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "theorize", Actor: g.Actor,
		Args: Args{Text: "Токе подменил запись"}})
	if got.Refused {
		t.Fatalf("гипотеза отвергнута: %s", got.Refusal)
	}
	if len(g.Theories) != 1 || g.Theories[0] != "Токе подменил запись" {
		t.Errorf("гипотеза не записана: %v", g.Theories)
	}
	if len(got.Costs) != 0 {
		t.Errorf("рассуждение стоило %v", got.Costs)
	}
	if g.DB.Clocks["c_suspicion"].Filled != 0 {
		t.Error("гипотеза тикнула часы")
	}
}

var _ = store.FactID("")
