package e2e

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core"
	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/dice"
	"github.com/kliuchnikovv/dnd/rules/threshold"
	"github.com/kliuchnikovv/dnd/store"
)

// Здесь сходятся обе половины границы из ADR-0001: парсер предлагает форму,
// ядро судит. Проверяется вторая половина — что подсказка над костью
// бесправна. По отдельности это уже доказано (DifficultyFor не читает интент;
// подсказка живёт в core.Probe и в Intent не попадает), но собрать цепочку
// целиком стоит здесь: разъехаться она может в любом звене.

// probeGame — тело с двумя авторскими гейтами разной сложности на разные
// глаголы. Подсказка выбирает между ними форму, и видно, что порог приходит
// от гейта и позиции, а не от неё.
func probeThresholdGame(t *testing.T) *core.Game {
	t.Helper()
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Name: "Тело Халдена",
		Kind: store.EntityThing, Node: "n_quay"}
	db.CharactersMap()["pc"] = &store.Character{ID: "pc", Grit: 3,
		Sheet: []byte(`{"attrs":{"mind":0},"tags":[]}`)}
	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "борозда"}
	db.Facts["f_wound"] = store.Fact{ID: "f_wound", Key: "рана"}
	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_body", Mandatory: false,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "easy"},
	}}
	db.Holders["f_wound"] = []store.FactHolder{{
		FactID: "f_wound", HolderID: "e_body", Mandatory: false,
		Gate: store.Gate{Verbs: []string{"grapple"}, Threshold: "hard"},
	}}
	return core.NewGame(core.Config{
		DB: db, Rules: threshold.New(), Dice: dice.NewSource(1).Stream("resolve"),
		Truth:   accusation.NewTruth("who", "how", "when", "why"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

// Порог импровизации совпадает с тем, что говорит DifficultyFor о позиции, —
// и ни с чем другим. Это то самое место, где раньше решал живой Мастер.
func TestImprovisedProbeTakesThresholdFromPositionNotFromTheHint(t *testing.T) {
	for _, hint := range []core.VerbClass{"", core.ClassAttack, core.ClassSkill} {
		g := probeThresholdGame(t)
		in, ok := g.MatchProbeAs(core.Probe{Text: "берусь за тело", Class: hint})
		if !ok {
			t.Fatalf("подсказка %q: проба не легла на авторскую цель", hint)
		}
		view := g.SceneView(in)
		want := threshold.ThresholdFor(threshold.DifficultyFor(in, view))

		res := g.Apply(in)
		if res.Res == nil {
			t.Fatalf("подсказка %q: проба не дошла до броска", hint)
		}
		if res.Res.Log.Threshold != want {
			t.Errorf("подсказка %q: порог %d, а функция от позиции говорит %d",
				hint, res.Res.Log.Threshold, want)
		}
	}
}

// Подсказка выбрала другую форму — и порог сменился потому, что у ДРУГОГО
// гейта другая авторская сложность, а не потому, что «attack звучит тяжелее».
// Разница видна только через авторские данные, и в этом всё дело.
func TestHintChangesTheFormAndTheAuthorStillSetsTheThreshold(t *testing.T) {
	plain := probeThresholdGame(t)
	inPlain, ok := plain.MatchProbeAs(core.Probe{Text: "берусь за тело"})
	if !ok || inPlain.Verb != "examine" {
		t.Fatalf("без подсказки проба легла как %q", inPlain.Verb)
	}
	hinted := probeThresholdGame(t)
	inHinted, ok := hinted.MatchProbeAs(core.Probe{Text: "берусь за тело", Class: core.ClassAttack})
	if !ok || inHinted.Verb != "grapple" {
		t.Fatalf("с подсказкой attack проба легла как %q", inHinted.Verb)
	}

	got := plain.Apply(inPlain).Res.Log.Threshold
	if got != threshold.ThresholdEasy {
		t.Errorf("порог examine %d, а автор написал easy (%d)", got, threshold.ThresholdEasy)
	}
	got = hinted.Apply(inHinted).Res.Log.Threshold
	if got != threshold.ThresholdHard {
		t.Errorf("порог grapple %d, а автор написал hard (%d)", got, threshold.ThresholdHard)
	}
}
