package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/scenarios/deduction/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

type nilRules struct{}

func (nilRules) Resolve(Intent, SceneView, Dice) Resolution { return Resolution{} }

type nilDice struct{}

func (nilDice) D20() int          { return 10 }
func (nilDice) Roll(int, int) int { return 3 }

func testGame() *Game {
	db := store.NewDB()
	db.Entities["e_toke"] = store.Entity{ID: "e_toke", Kind: store.EntityNPC, Node: "n_quay"}
	db.CharactersMap()["pc"] = &store.Character{ID: "pc", Grit: 3, Harm: 0}
	db.Clocks["c_suspicion"] = &store.Clock{
		ID: "c_suspicion", Segments: 2, TickPolicy: "on_cost",
		OnFill: store.Consequence{FlavourKey: "clock.filled", HostileTo: []store.EntityID{"e_toke"}},
	}
	db.Locations["n_quay"] = store.Location{ID: "n_quay"}
	return NewGame(Config{
		DB:      db,
		Rules:   nilRules{},
		Dice:    nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{"clock.filled": "Прилив забрал следы."},
		Start:   "n_quay",
		Actor:   "pc",
	})
}

func TestMutationsAreAppliedByName(t *testing.T) {
	g := testGame()
	g.applyMutations([]Mutation{
		{Kind: MutResource, Target: "grit", Delta: -1},
		{Kind: MutHarm, Target: "pc", Delta: 1},
		{Kind: MutDisposition, Target: "e_toke", Delta: -1},
	})
	if got := g.DB.CharactersMap()["pc"].Grit; got != 2 {
		t.Errorf("grit = %d, ожидалось 2", got)
	}
	if got := g.DB.CharactersMap()["pc"].Harm; got != 1 {
		t.Errorf("harm = %d, ожидалось 1", got)
	}
	if got := g.D.Disposition("e_toke"); got != -1 {
		t.Errorf("disposition = %d, ожидалось -1", got)
	}
}

func TestUnknownResourceIsIgnoredNotPanicked(t *testing.T) {
	// Имена ресурсов даёт система правил. Ядро их не интерпретирует и не
	// обязано знать: незнакомое имя не должно ронять прогон.
	g := testGame()
	g.applyMutations([]Mutation{{Kind: MutResource, Target: "slot_3", Delta: -1}})
	if g.DB.CharactersMap()["pc"].Grit != 3 {
		t.Error("незнакомый ресурс задел grit")
	}
}

func TestEveryCostKindExecutes(t *testing.T) {
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	for _, kind := range AllCostKinds() {
		g := testGame()
		before := snapshot(g)
		g.executeCosts([]CostKind{kind}, in)
		if kind == CostHalfEffect || kind == CostResourceSpent || kind == CostFalseLead {
			// Эти три меняют не состояние мира, а исход хода: их эффект
			// проверяется в TurnResult (Task 13).
			continue
		}
		if snapshot(g) == before {
			t.Errorf("цена %q не изменила состояние — не исполнена", kind)
		}
	}
}

func TestTickClockCostFiresConsequence(t *testing.T) {
	g := testGame()
	in := Intent{Verb: "question", Args: Args{Target: "e_toke"}}
	if got := g.executeCosts([]CostKind{CostTickClock}, in); len(got) != 0 {
		t.Fatalf("часы сработали на первом тике из двух: %v", got)
	}
	got := g.executeCosts([]CostKind{CostTickClock}, in)
	if len(got) != 1 || got[0].FlavourKey != "clock.filled" {
		t.Fatalf("последствие не сработало: %v", got)
	}
	g.applyConsequences(got)
	if g.D.Disposition("e_toke") >= 0 {
		t.Error("HostileTo не понизил расположение")
	}
}

func TestFlavourFallsBackToKey(t *testing.T) {
	g := testGame()
	if got := g.Flavour("clock.filled"); got != "Прилив забрал следы." {
		t.Errorf("текст не найден: %q", got)
	}
	// Отсутствующий ключ виден сразу, а не молча превращается в пустую строку.
	if got := g.Flavour("нет.такого.ключа"); got != "[нет.такого.ключа]" {
		t.Errorf("отсутствующий ключ = %q, ожидалось [нет.такого.ключа]", got)
	}
}

func snapshot(g *Game) [4]int {
	c := g.DB.CharactersMap()["pc"]
	return [4]int{c.Grit, c.Harm, g.D.Disposition("e_toke") + g.Debts["e_toke"],
		g.DB.Clocks["c_suspicion"].Filled + boolInt(g.Detected)}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Недоверенные виды мутаций доверенного обработчика не имеют. Попав в путь
// правил, они не меняют ни состояние, ни канон: единственный вход для них —
// ProposeMutation, и обойти его через Resolution.Mutations нельзя.
func TestProposalKindsAreNotAppliedByRulesPath(t *testing.T) {
	g := testGame()
	before := snapshot(g)
	g.applyMutations([]Mutation{
		{Kind: MutCanonAmbient, Target: "погода", Text: "морось с моря"},
		{Kind: MutWorldEvent, Target: "n_quay"},
	})
	if got := snapshot(g); got != before {
		t.Errorf("недоверенный вид изменил состояние: %v → %v", before, got)
	}
	if got := g.Canon(); len(got) != 0 {
		t.Errorf("путь правил записал канон: %+v", got)
	}
}

// Дальше — недоверенный вход. ProposeMutation отличается от пути правил
// главным: он всегда даёт вердикт. Молчание здесь было бы дырой, потому что
// предлагает тот, кому ядро не доверяет.

func canonProposal(topic, text string, turn int) Mutation {
	return Mutation{Kind: MutCanonAmbient, Target: topic, Text: text, Delta: turn}
}

func TestProposedCanonIsApplied(t *testing.T) {
	g := testGame()
	app, ref := g.ProposeMutation(canonProposal("погода", "морось с моря", 3))
	if ref.Refused() {
		t.Fatalf("валидное предложение отвергнуто: %q", ref.Reason)
	}
	if app.Kind != MutCanonAmbient || app.Text != "морось с моря" {
		t.Errorf("применено не то: %+v", app)
	}
	if got, ok := g.CanonGet("погода"); !ok || got != "морось с моря" {
		t.Errorf("канон не записан: %q (%v)", got, ok)
	}
}

// Топик из пространства имён фактов дела отвергается. Иначе ambient-канон
// стал бы вторым способом выдать факт — тем самым, который гейтит
// fact_holders.
func TestProposedCanonRejectsCaseFactNamespace(t *testing.T) {
	g := testGame()
	g.DB.Facts["f_toke_lied"] = store.Fact{
		ID: "f_toke_lied", Key: "Токе соврал о ночи прилива",
	}
	for _, topic := range []string{"f_toke_lied", "F_Toke_Lied", "f_чего_угодно",
		"Токе соврал о ночи прилива"} {
		app, ref := g.ProposeMutation(canonProposal(topic, "что-то", 1))
		if !ref.Refused() {
			t.Errorf("топик %q прошёл как канон: %+v", topic, app)
		}
	}
	if got := g.Canon(); len(got) != 0 {
		t.Errorf("факт дела уехал в канон: %+v", got)
	}
}

func TestProposedCanonRejectsEmpty(t *testing.T) {
	g := testGame()
	if _, ref := g.ProposeMutation(canonProposal("погода", "   ", 1)); !ref.Refused() {
		t.Error("пустой текст стал каноном")
	}
	if _, ref := g.ProposeMutation(canonProposal("  ", "морось", 1)); !ref.Refused() {
		t.Error("пустая тема стала каноном")
	}
}

// Повтор того же — не ошибка, а идемпотентность: реплей и переспрос обязаны
// давать один ответ. Иной текст на занятой теме — отказ, потому что канон
// держит слово.
func TestProposedCanonIsIdempotentAndRefusesConflict(t *testing.T) {
	g := testGame()
	g.ProposeMutation(canonProposal("погода", "морось с моря", 1))

	app, ref := g.ProposeMutation(canonProposal("Погода", "  морось с моря ", 5))
	if ref.Refused() {
		t.Errorf("повтор того же отвергнут: %q", ref.Reason)
	}
	if app.Text != "морось с моря" {
		t.Errorf("повтор вернул %q", app.Text)
	}

	if _, ref := g.ProposeMutation(canonProposal("погода", "сухо и ясно", 6)); !ref.Refused() {
		t.Error("конфликтный текст переписал канон")
	}
	if got, _ := g.CanonGet("погода"); got != "морось с моря" {
		t.Errorf("канон сдвинулся: %q", got)
	}
}

// Числовые виды — прерогатива правил. Предложить их нельзя: иначе нарратор
// раздавал бы ранения и двигал часы.
func TestProposedNumericKindsAreRefused(t *testing.T) {
	g := testGame()
	for _, m := range []Mutation{
		{Kind: MutHarm, Target: "pc", Delta: 1},
		{Kind: MutResource, Target: "grit", Delta: -1},
		{Kind: MutClock, Target: "c_suspicion", Delta: 1},
		{Kind: MutDisposition, Target: "e_toke", Delta: -1},
		{Kind: MutPosition, Delta: -1},
	} {
		before := snapshot(g)
		app, ref := g.ProposeMutation(m)
		if !ref.Refused() {
			t.Errorf("вид %q прошёл недоверенным входом: %+v", m.Kind, app)
		}
		if got := snapshot(g); got != before {
			t.Errorf("вид %q изменил состояние до отказа: %v → %v", m.Kind, before, got)
		}
	}
}

// Незнакомый вид получает отказ, а не тишину: этим недоверенный вход и
// отличается от applyMutations.
func TestProposedUnknownKindIsRefusedNotIgnored(t *testing.T) {
	g := testGame()
	if _, ref := g.ProposeMutation(Mutation{Kind: "выдать_правду", Target: "toke"}); !ref.Refused() {
		t.Error("незнакомый вид прошёл молча")
	}
	// MutWorldEvent объявлен формой, обработчика нет — значит тоже отказ, а не
	// вид, который «как-нибудь» применится.
	if _, ref := g.ProposeMutation(Mutation{Kind: MutWorldEvent, Target: "n_quay"}); !ref.Refused() {
		t.Error("MutWorldEvent применился без обработчика")
	}
}

// Место, о котором рассказали, становится известным через границу — и только
// смежное текущему узлу. Иначе одна болтливая реплика раскрыла бы карту, и
// разведка перестала бы быть занятием.
func TestProposedPlaceMustBeAdjacent(t *testing.T) {
	g := placesGame() // из core/places_test.go, игрок на n_quay
	app, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_warehouse"})
	if ref.Refused() {
		t.Fatalf("смежное место отвергнуто: %q", ref.Reason)
	}
	if app.Kind != MutPlaceKnown || app.Target != "n_warehouse" {
		t.Errorf("применено не то: %+v", app)
	}
	if !g.KnowsPlace("n_warehouse") {
		t.Error("место не стало известным")
	}

	// Таверна смежна кузнице, а не пристани: отсюда о ней рассказать нельзя.
	if _, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_tavern"}); !ref.Refused() {
		t.Error("несмежное место прошло")
	}
	// Несуществующего места не бывает даже в рассказе.
	if _, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_нет"}); !ref.Refused() {
		t.Error("несуществующий узел прошёл")
	}
}

// Знание места не касается улик. Инвариант §4 спеки: место — публичная
// география, и попасть в таблицу фактов оно не вправе даже случайно.
func TestProposedPlaceTouchesNoKnowledge(t *testing.T) {
	g := placesGame()
	before := len(g.DB.Knowledge)
	g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_warehouse"})
	if len(g.DB.Knowledge) != before {
		t.Errorf("рассказ о месте написал в party_knowledge: %+v", g.DB.Knowledge)
	}
	if len(g.K.TopicBank()) != 0 {
		t.Errorf("место появилось в банке тем: %v", g.K.TopicBank())
	}
}

// Повтор — идемпотентность, а не ошибка: персонаж вправе рассказать дорогу
// второй раз, и это не отказ.
func TestProposedPlaceRepeatIsApplied(t *testing.T) {
	g := placesGame()
	g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_forge"})
	app, ref := g.ProposeMutation(Mutation{Kind: MutPlaceKnown, Target: "n_forge"})
	if ref.Refused() || app.Kind != MutPlaceKnown {
		t.Errorf("повтор отвергнут: %q / %+v", ref.Reason, app)
	}
}
