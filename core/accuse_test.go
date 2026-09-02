package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

func accuseGame() *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	for _, f := range []store.FactID{"f_who", "f_how", "f_when", "f_why", "f_wrong"} {
		db.Facts[f] = store.Fact{ID: f}
	}
	return NewGame(Config{
		DB: db, Rules: nilRules{}, Dice: nilDice{},
		Truth: accusation.NewTruth("toke", "cord", "night", "audit"),
		Tokens: []TokenGrant{
			{Slot: "who", Token: "toke", Fact: "f_who"},
			{Slot: "who", Token: "ivar", Fact: "f_wrong"},
			{Slot: "how", Token: "cord", Fact: "f_how"},
			{Slot: "how", Token: "ivar", Fact: "f_wrong"},
			{Slot: "when", Token: "night", Fact: "f_when"},
			{Slot: "when", Token: "ivar", Fact: "f_wrong"},
			{Slot: "why", Token: "audit", Fact: "f_why"},
			{Slot: "why", Token: "ivar", Fact: "f_wrong"},
		},
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func learnAll(g *Game) {
	for _, f := range []store.FactID{"f_who", "f_how", "f_when", "f_why"} {
		g.K.Learn(f, "e_bern")
	}
}

func TestTokenAvailableOnlyUnderCollectedFact(t *testing.T) {
	g := accuseGame()
	if got := g.AvailableTokens("who"); len(got) != 0 {
		t.Fatalf("токены доступны без фактов: %v", got)
	}
	g.K.Learn("f_who", "e_bern")
	got := g.AvailableTokens("who")
	if len(got) != 1 || got[0] != "toke" {
		t.Errorf("доступные токены = %v, ожидалось [toke]", got)
	}
}

func TestAccusationWithUnavailableTokenIsRefused(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	form := accusation.Form{Who: "sigrid", How: "cord", When: "night", Why: "audit"}
	got := g.Accuse(form)
	if !got.Refused {
		t.Error("принят токен, не подкреплённый фактом")
	}
	if g.Attempts != 0 {
		t.Error("отклонённая форма засчитана попыткой")
	}
}

func TestCorrectAccusationSucceeds(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	got := g.Accuse(accusation.Form{Who: "toke", How: "cord", When: "night", Why: "audit"})
	if got.Refused {
		t.Fatalf("верное обвинение отвергнуто: %s", got.Refusal)
	}
	if !got.Correct {
		t.Error("верное обвинение признано ошибочным")
	}
	if got.Attempt != 1 {
		t.Errorf("номер попытки = %d, ожидался 1", got.Attempt)
	}
}

func TestEveryAttemptCostsAClockTick(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	g.K.Learn("f_wrong", "e_bern")
	before := g.DB.Clocks["c_suspicion"].Filled
	g.Accuse(accusation.Form{Who: "ivar", How: "cord", When: "night", Why: "audit"})
	if got := g.DB.Clocks["c_suspicion"].Filled; got != before+1 {
		t.Errorf("часы = %d, ожидалось %d — иначе слоты брутфорсятся", got, before+1)
	}
	if g.Attempts != 1 {
		t.Errorf("счётчик попыток = %d, ожидался 1", g.Attempts)
	}
}

func TestPartialMatchLeaksNothing(t *testing.T) {
	// Три верных слота и ноль верных должны быть неразличимы по результату.
	g := accuseGame()
	learnAll(g)
	g.K.Learn("f_wrong", "e_bern")

	three := g.Accuse(accusation.Form{Who: "ivar", How: "cord", When: "night", Why: "audit"})
	zero := g.Accuse(accusation.Form{Who: "ivar", How: "ivar", When: "ivar", Why: "ivar"})

	if three.Correct != zero.Correct {
		t.Fatal("частичное совпадение отличимо от полного промаха")
	}
	if three.Refusal != zero.Refusal {
		t.Errorf("тексты отказа различаются: %q против %q", three.Refusal, zero.Refusal)
	}
}

func TestIncompleteFormIsRefusedWithoutCost(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	before := g.DB.Clocks["c_suspicion"].Filled
	got := g.Accuse(accusation.Form{Who: "toke"})
	if !got.Refused {
		t.Error("неполная форма принята")
	}
	if g.DB.Clocks["c_suspicion"].Filled != before {
		t.Error("неполная форма стоила тика")
	}
}

// Развязка — речь игрока, собранная из его же выводов. Строки берутся у тех
// фактов, чьи токены он поставил в слоты, в порядке who → how → when → why.
// Ни одной посылки, которой он не собрал, в тексте быть не может.
func TestSummationIsBuiltFromTheClausesOfPlacedFacts(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	for id, clause := range map[store.FactID]string{
		"f_who":   "…и это был Токе.",
		"f_how":   "…потому что шнур с печатью бывает только у него.",
		"f_when":  "…в ночь перед приливом.",
		"f_why":   "…из-за недостачи, которую вскрыл бы аудит.",
		"f_wrong": "…и это был Ивар.",
	} {
		f := g.DB.Facts[id]
		f.SummationClause = clause
		g.DB.Facts[id] = f
	}

	res := g.Accuse(accusation.Form{Who: "toke", How: "cord", When: "night", Why: "audit"})
	if !res.Correct {
		t.Fatal("верное обвинение не принято")
	}
	want := []string{
		"…и это был Токе.",
		"…потому что шнур с печатью бывает только у него.",
		"…в ночь перед приливом.",
		"…из-за недостачи, которую вскрыл бы аудит.",
	}
	if len(res.Summation) != len(want) {
		t.Fatalf("развязка из %d строк, ожидалось %d: %v", len(res.Summation), len(want), res.Summation)
	}
	for i, w := range want {
		if res.Summation[i] != w {
			t.Errorf("строка %d: %q, ожидалось %q", i, res.Summation[i], w)
		}
	}
}

// Ошибочное обвинение развязки не даёт: иначе игра объясняла бы решение тому,
// кто его не нашёл.
func TestWrongAccusationHasNoSummation(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	g.K.Learn("f_wrong", "e_bern")
	res := g.Accuse(accusation.Form{Who: "ivar", How: "cord", When: "night", Why: "audit"})
	if res.Correct {
		t.Fatal("ошибочное обвинение принято за верное")
	}
	if len(res.Summation) != 0 {
		t.Errorf("развязка выдана при ошибке: %v", res.Summation)
	}
}

// Один факт может стоять за несколькими слотами. В речи он звучит один раз:
// повторённая четырежды строка читается как сбой, а не как вывод.
func TestSummationSaysEachClauseOnce(t *testing.T) {
	g := accuseGame()
	learnAll(g)
	for _, id := range []store.FactID{"f_who", "f_how", "f_when", "f_why"} {
		f := g.DB.Facts[id]
		f.SummationClause = "…одна и та же мысль."
		g.DB.Facts[id] = f
	}
	res := g.Accuse(accusation.Form{Who: "toke", How: "cord", When: "night", Why: "audit"})
	if len(res.Summation) != 1 {
		t.Errorf("клауза повторена %d раз: %v", len(res.Summation), res.Summation)
	}
}
