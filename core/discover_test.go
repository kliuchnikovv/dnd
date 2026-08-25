package core

import (
	"testing"

	"github.com/kliuchnikovv/dnd/core/accusation"
	"github.com/kliuchnikovv/dnd/store"
)

// discoverGame строит дело для проверки topic-less обнаружения фактов:
// examine/search/stake_out/tail находят факты у цели без темы, question
// остаётся topic-driven (защита от угадывания).
func discoverGame(out Outcome) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_forge"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities["e_body"] = store.Entity{ID: "e_body", Kind: store.EntityThing, Node: "n_quay"}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}

	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "ligature"}
	db.Facts["f_seal_cord"] = store.Fact{ID: "f_seal_cord", Key: "seal_cord"}
	db.Facts["f_hidden"] = store.Fact{ID: "f_hidden", Key: "hidden"}

	// f_ligature — первый факт, который должен обнаружить examine: без
	// requires, доступен глаголу examine, mandatory (провал брoска не
	// мешает обнаружению).
	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal"},
	}}
	// f_seal_cord — второй факт: требует f_ligature, тоже под examine.
	db.Holders["f_seal_cord"] = []store.FactHolder{{
		FactID: "f_seal_cord", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal",
			Requires: store.RequireAll("f_ligature")},
	}}
	// f_hidden — держится тем же e_body, но под глаголом search, не examine:
	// examine не должен его выдавать.
	db.Holders["f_hidden"] = []store.FactHolder{{
		FactID: "f_hidden", HolderID: "e_body", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"search"}, Threshold: "normal"},
	}}

	db.Clocks["c_suspicion"] = &store.Clock{ID: "c_suspicion", Segments: 6, TickPolicy: "on_cost"}
	return NewGame(Config{
		DB: db, Rules: fixedRules{out}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestExamineWithoutTopicDeliversMandatoryFactOnFailedRoll(t *testing.T) {
	// Кубик всегда проваливает: mandatory-факт обязан выдаться без броска.
	g := discoverGame(OutcomeFail)
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	if got.Refused {
		t.Fatalf("examine без темы отвергнут: %s", got.Refusal)
	}
	if got.Res != nil {
		t.Error("mandatory-факт прошёл через бросок")
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_ligature" {
		t.Fatalf("факт не выдан: %v", got.Learned)
	}
	if !g.K.Knows("f_ligature") {
		t.Error("факт не записан в party_knowledge")
	}
}

func TestSecondExamineDeliversNextFactProgressively(t *testing.T) {
	g := discoverGame(OutcomeFail)
	g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}}) // выдаёт f_ligature

	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	if got.Refused {
		t.Fatalf("второй examine отвергнут: %s", got.Refusal)
	}
	if len(got.Learned) != 1 || got.Learned[0].Fact != "f_seal_cord" {
		t.Fatalf("второй examine должен выдать f_seal_cord: %v", got.Learned)
	}
	if !g.K.Knows("f_seal_cord") {
		t.Error("f_seal_cord не записан в party_knowledge")
	}
}

func TestExamineDoesNotDeliverFactGatedByAnotherVerb(t *testing.T) {
	g := discoverGame(OutcomeFail)
	g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})

	// Оба факта под examine выданы; f_hidden держится тем же телом, но
	// доступен только search — третий examine не должен его выдать.
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	for _, l := range got.Learned {
		if l.Fact == "f_hidden" {
			t.Error("examine выдал факт, гейт которого требует другого глагола")
		}
	}
	if g.K.Knows("f_hidden") {
		t.Error("f_hidden не должен быть известен после examine")
	}
}

func TestTopiclessDeliveryFlavourKey(t *testing.T) {
	g := discoverGame(OutcomeFail)
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}})
	want := "examine.e_body.f_ligature"
	if got.FlavourKey != want {
		t.Errorf("FlavourKey = %q, хотим %q", got.FlavourKey, want)
	}
}

// --- Регрессия: анти-угадывание для topic-driven глаголов не задето. ---

func TestRegressionQuestionAboutUnknownFactStillRefused(t *testing.T) {
	g := discoverGame(OutcomeSuccess)
	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_body", Topic: "f_seal_cord"}})
	if !got.Refused {
		t.Fatal("question о неизвестном факте прошёл — защита от угадывания дырявая")
	}
}

func TestRegressionQuestionAboutKnownTopicStillWorks(t *testing.T) {
	g := discoverGame(OutcomeSuccess)
	// question использует тот же topic-driven путь: держатель обязан быть
	// в gate.verbs для question, поэтому заводим отдельный факт с этим гейтом.
	db := g.DB
	db.Facts["f_asked"] = store.Fact{ID: "f_asked", Key: "asked"}
	db.Holders["f_asked"] = []store.FactHolder{{
		FactID: "f_asked", HolderID: "e_body", Mandatory: false,
		Gate: store.Gate{Verbs: []string{"question"}, Threshold: "normal"},
	}}
	g.K.Learn("f_asked", "e_someone") // тема уже в party_knowledge

	got := g.Apply(Intent{Verb: "question", Args: Args{Target: "e_body", Topic: "f_asked"}})
	if got.Refused {
		t.Fatalf("question по известной теме отвергнут: %s", got.Refusal)
	}
}

// Кто произносит выданный факт — решает ядро, и решение это одно на всю игру.
// Раньше его принимали два места над доменом, каждое по своему признаку «в
// ходу есть Learned»: подпись в презентации и самоглушение актёра. Признак
// слишком грубый — труп не говорит, а стражник говорит.

// factHolderGame: тот же факт держат человек в сцене, человек в другом узле и
// вещь. Глагол один, чтобы различие шло от держателя, а не от хода.
func factHolderGame(holder store.EntityID, kind store.EntityKind, node store.NodeID) *Game {
	db := store.NewDB()
	db.Locations["n_quay"] = store.Location{ID: "n_quay", Adjacent: []store.NodeID{"n_forge"}}
	db.Locations["n_forge"] = store.Location{ID: "n_forge", Adjacent: []store.NodeID{"n_quay"}}
	db.Entities[holder] = store.Entity{ID: holder, Kind: kind, Node: node}
	db.Characters["pc"] = &store.Character{ID: "pc", Grit: 3}
	db.Facts["f_ligature"] = store.Fact{ID: "f_ligature", Key: "ligature"}
	db.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: holder, Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal"},
	}}
	return NewGame(Config{
		DB: db, Rules: fixedRules{OutcomeSuccess}, Dice: nilDice{},
		Truth:   accusation.NewTruth("toke", "cord", "night", "audit"),
		Flavour: map[string]string{}, Start: "n_quay", Actor: "pc",
	})
}

func TestSpokenByNamesTheHolderWhoCanSpeak(t *testing.T) {
	g := factHolderGame("e_bern", store.EntityNPC, "n_quay")
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_bern"}})
	if len(got.Learned) != 1 {
		t.Fatalf("факт не выдан: %v", got.Learned)
	}
	if got.SpokenBy != "e_bern" {
		t.Errorf("SpokenBy = %q, человек в сцене обязан произнести своё", got.SpokenBy)
	}
}

func TestSpokenByIsEmptyForThingsAndForTheAbsent(t *testing.T) {
	// Вещь не говорит: факт от трупа, замка или книги учёта остаётся прозой.
	thing := factHolderGame("e_body", store.EntityThing, "n_quay")
	if got := thing.Apply(Intent{Verb: "examine", Args: Args{Target: "e_body"}}); got.SpokenBy != "" {
		t.Errorf("вещь заговорила: SpokenBy = %q", got.SpokenBy)
	}
	// Отсутствующий не говорит тоже, даже если факт достался от него: реплику
	// произносит тот, кто здесь, иначе голос приходит из пустого места.
	away := factHolderGame("e_ivar", store.EntityNPC, "n_forge")
	away.DB.Holders["f_ligature"] = []store.FactHolder{{
		FactID: "f_ligature", HolderID: "e_ivar", Mandatory: true,
		Gate: store.Gate{Verbs: []string{"examine"}, Threshold: "normal"},
	}}
	got := away.Apply(Intent{Verb: "examine", Args: Args{Target: "e_ivar"}})
	if got.SpokenBy != "" {
		t.Errorf("голос пришёл из другого узла: SpokenBy = %q", got.SpokenBy)
	}
}

// Ход без факта никого не заставляет говорить: SpokenBy отвечает за
// произнесение выданного, а не за то, кто вообще рядом.
func TestSpokenByIsEmptyWithoutLearnedFact(t *testing.T) {
	g := factHolderGame("e_bern", store.EntityNPC, "n_quay")
	g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_bern"}}) // выбирает единственный факт
	got := g.Apply(Intent{Verb: "examine", Args: Args{Target: "e_bern"}})
	if len(got.Learned) != 0 {
		t.Fatalf("второй осмотр что-то нашёл: %v", got.Learned)
	}
	if got.SpokenBy != "" {
		t.Errorf("SpokenBy = %q на ходу без находки", got.SpokenBy)
	}
}
