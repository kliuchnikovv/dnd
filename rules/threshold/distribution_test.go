package threshold

import (
	"fmt"
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/dice"
)

func TestOutcomeDistributionMatchesSpec(t *testing.T) {
	cases := []struct {
		mod                             int
		gate                            string
		successPlus, partial, cleanFail int // из 20 граней
	}{
		// Каноничная шкала: medium 15, hard 20. Окно «частично» — 4 пункта.
		{4, "normal", 10, 4, 6}, // 50% / 20% / 30%
		{6, "normal", 12, 4, 4}, // 60% / 20% / 20%
		{4, "hard", 5, 4, 11},   // 25% / 20% / 55%
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("mod%+d_%s", c.mod, c.gate), func(t *testing.T) {
			view := core.SceneView{
				GateThreshold: c.gate,
				Grit:          0, // конвертация grit исказила бы распределение
				Sheet:         []byte(fmt.Sprintf(`{"attrs":{"mind":%d},"tags":[]}`, c.mod)),
			}
			var successPlus, partial, fail int
			for die := 1; die <= 20; die++ {
				got := New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(die))
				switch got.Class {
				case core.OutcomeCrit, core.OutcomeSuccess:
					successPlus++
				case core.OutcomePartial:
					partial++
				default:
					fail++
				}
			}
			if successPlus != c.successPlus || partial != c.partial || fail != c.cleanFail {
				t.Errorf("распределение %d/%d/%d из 20, ожидалось %d/%d/%d",
					successPlus, partial, fail, c.successPlus, c.partial, c.cleanFail)
			}
		})
	}
}

func TestPartialStaysNearTwentyPercentAcrossModifiers(t *testing.T) {
	// Окно «частично» шириной 4 пункта не зависит от прокачки — это фича:
	// доля fail-forward постоянна.
	for mod := 0; mod <= 8; mod++ {
		view := core.SceneView{
			GateThreshold: "normal",
			Sheet:         []byte(fmt.Sprintf(`{"attrs":{"mind":%d},"tags":[]}`, mod)),
		}
		partial := 0
		for die := 1; die <= 20; die++ {
			if New().Resolve(core.Intent{Verb: "question"}, view, dice.Fixed(die)).Class == core.OutcomePartial {
				partial++
			}
		}
		if partial != 4 {
			t.Errorf("модификатор +%d: частично %d/20, ожидалось 4/20", mod, partial)
		}
	}
}
