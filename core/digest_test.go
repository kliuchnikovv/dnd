package core

import (
	"strings"
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// digestGame — дело для проверки дайджеста состояния. Инвентарь, часы, факты и
// узел заданы явно, а правда дела намеренно состоит из строк, которых нет ни
// среди ключей фактов, ни среди имён предметов: так тест на утечку truth ловит
// именно утечку, а не совпадение по подстроке.
func digestGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Name: "Пристань",
		Adjacent: []store.NodeID{"n_forge"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Name: "Кузница"}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Kind: store.EntityThing, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Harm: 1, Grit: 2}

	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "тело на складе"}
	db.Facts["f_seal"] = store.Fact{ID: "f_seal", Key: "шнур от печати"}

	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal"},
	}}

	db.Items["i_warrant"] = store.Item{ID: "i_warrant", Name: "предписание", Kind: "credential"}
	db.Items["i_lantern"] = store.Item{ID: "i_lantern", Name: "фонарь", Kind: store.ToolKind}

	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Name: "подозрение",
		Segments: 6, Filled: 2, TickPolicy: "on_cost"}

	return NewGame(Config{
		DB: db, Rules: fixedRules{out: OutcomeSuccess}, Dice: nilDice{},
		// Правда дела — маркерные строки: если хоть одна всплывёт в дайджесте,
		// это утечка, а не совпадение.
		Truth:   accusation.NewTruth("ВИНОВНЫЙ_ПИСАРЬ", "УДАВКА", "ПОЛНОЧЬ", "РАСТРАТА"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

// Дайджест собирается из фикстуры: то, что парти проверяемо несёт, где стоит,
// чем ранена, что знает и как заполнены часы.
func TestStateDigestReadsFixture(t *testing.T) {
	g := digestGame()
	g.Acquire("i_warrant")

	d := StateDigest(g, TurnResult{})

	if len(d.Carried) != 1 || d.Carried[0] != "предписание" {
		t.Errorf("инвентарь не тот: %v", d.Carried)
	}
	if d.Harm != 1 || d.Grit != 2 {
		t.Errorf("раны/стойкость не те: harm=%d grit=%d", d.Harm, d.Grit)
	}
	if d.Node != "Пристань" {
		t.Errorf("узел не тот: %q", d.Node)
	}
	if len(d.Clocks) != 1 || d.Clocks[0].Name != "подозрение" ||
		d.Clocks[0].Filled != 2 || d.Clocks[0].Segments != 6 {
		t.Errorf("часы не те: %+v", d.Clocks)
	}
	// Пока ничего не узнали — известных фактов нет.
	if len(d.Known) != 0 {
		t.Errorf("известное взялось из ниоткуда: %v", d.Known)
	}
}

// Дайджест меняется вслед за состоянием: ход, открывший факт и передвинувший
// игрока, обязан отразиться и в known, и в исходе, и в узле.
func TestStateDigestFollowsState(t *testing.T) {
	g := digestGame()

	res := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	if len(res.Learned) == 0 {
		t.Fatalf("ход ничего не открыл: %+v", res)
	}
	g.MoveTo("n_forge")
	g.Acquire("i_lantern")

	d := StateDigest(g, res)

	if d.Node != "Кузница" {
		t.Errorf("дайджест не проследил за перемещением: %q", d.Node)
	}
	if !contains(d.Known, "тело на складе") {
		t.Errorf("открытый факт не попал в known: %v", d.Known)
	}
	if !contains(d.Carried, "фонарь") {
		t.Errorf("взятый предмет не попал в инвентарь: %v", d.Carried)
	}
	if !contains(d.Outcome.Learned, "тело на складе") {
		t.Errorf("исход хода не назвал открытое: %+v", d.Outcome)
	}
	// Тот же факт теперь известен — исход и known согласованы.
	if !contains(d.Known, "тело на складе") {
		t.Errorf("known отстал от исхода: %v", d.Known)
	}
}

// Дайджест — чистая функция: два вызова на одном состоянии дают одно и то же,
// и порядок известного/часов стабилен.
func TestStateDigestIsDeterministic(t *testing.T) {
	g := digestGame()
	g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})

	a := StateDigest(g, TurnResult{})
	b := StateDigest(g, TurnResult{})

	if strings.Join(a.Lines(), "\n") != strings.Join(b.Lines(), "\n") {
		t.Errorf("дайджест недетерминирован:\n%q\n%q", a.Lines(), b.Lines())
	}
}

// Правда дела в дайджест не течёт: пока парти сама не узнала виновного, способа
// и времени, ни одной маркерной строки truth в сводке быть не может.
func TestStateDigestDoesNotLeakTruth(t *testing.T) {
	g := digestGame()
	// Ход сделан, факт открыт — и всё равно правды дела в дайджесте нет.
	res := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})

	text := strings.Join(StateDigest(g, res).Lines(), "\n")
	for _, secret := range []string{"ВИНОВНЫЙ_ПИСАРЬ", "УДАВКА", "ПОЛНОЧЬ", "РАСТРАТА"} {
		if strings.Contains(text, secret) {
			t.Errorf("в дайджест утекла правда дела %q:\n%s", secret, text)
		}
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
